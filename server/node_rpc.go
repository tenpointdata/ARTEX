package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// Node RPC (E7, docs/DISTRIBUTED.md "Evidence on worker nodes"). The capture
// store and the evidence blob directory live on the control node, so a worker
// node asks the control node for every call that reads captured traffic:
//
//   - tool:   the traffic_* agent tools (traffic_search, traffic_get, ...);
//   - fact:   fact provenance (agent.FactTrafficLookup.ResponseContaining);
//   - exists: prove_goal's check that a bound exchange is still captured;
//   - record: report_finding with traffic_refs (evidence.Store.Record);
//   - bind:   bind_finding_traffic (evidence.Store.Bind);
//   - preview: get_finding_traffic's read of a bound body segment;
//   - assess: the S1 quote check of update_finding_report, run unchanged on
//     the node holding the bound bodies.
//
// record and bind run the control node's own evidence.Store, so the snapshot
// bodies land in the control node's evidence directory, where the UI, reports,
// verification and archives read them. Every agent-side check (the auto-bind
// setting, finding access, task provenance) still runs on the worker before the
// call, exactly as on a single process.
//
// Authentication: each request carries an HMAC-SHA256 under db.NodeRPCKey, a
// sub-key of ARTEX_SECRET_KEY. Every node of a deployment holds that key and
// agent processes never do (env_scrub.go), unlike the scope token, which rides
// in the agents' proxy URL. A request is refused when its signature is wrong,
// its timestamp is more than nodeRPCSkew away, or its nonce was seen before.
// The routes exist on the control node only (role=control).

const (
	nodeRPCPrefix   = "/api/node/rpc/"
	nodeRPCSkew     = 60 * time.Second
	nodeRPCMaxBody  = 1 << 20
	nodeRPCMaxReply = 8 << 20

	hdrNodeID    = "X-Artex-Node"
	hdrNodeTime  = "X-Artex-Time"
	hdrNodeNonce = "X-Artex-Nonce"
	hdrNodeSig   = "X-Artex-Signature"
)

// The request and reply bodies of each op. Shared by both ends.
type (
	nodeToolReq struct {
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	nodeToolReply struct {
		Result actool.Result `json:"result"`
	}
	nodeFactReq struct {
		Lines  []string `json:"lines"`
		FromTS int64    `json:"from_ts"`
		ToTS   int64    `json:"to_ts"`
	}
	nodeFactReply struct {
		ID string `json:"id"`
		OK bool   `json:"ok"`
	}
	nodeExistsReq struct {
		TrafficID string `json:"traffic_id"`
	}
	nodeExistsReply struct {
		Exists bool `json:"exists"`
	}
	nodeRecordReq struct {
		Input db.RecordFindingInput `json:"input"`
		Refs  []db.TrafficRef       `json:"refs"`
	}
	nodeBindReq struct {
		FindingID int64           `json:"finding_id"`
		Refs      []db.TrafficRef `json:"refs"`
	}
	nodeAssessReq struct {
		FindingID int64    `json:"finding_id"`
		POE       int      `json:"poe"`
		Quotes    []string `json:"quotes"`
	}
	nodeAssessReply struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	nodeRPCError struct {
		Error string `json:"error"`
	}
)

// nodeRPCSignature is the MAC over everything that defines one request.
func nodeRPCSignature(key []byte, method, path, nodeID, ts, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	m := hmac.New(sha256.New, key)
	for _, part := range []string{method, path, nodeID, ts, nonce, hex.EncodeToString(sum[:])} {
		m.Write([]byte(part))
		m.Write([]byte{'\n'})
	}
	return hex.EncodeToString(m.Sum(nil))
}

// signNodeRPC sets the authentication headers of req for body.
func signNodeRPC(req *http.Request, key []byte, nodeID string, body []byte, now time.Time) error {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	ts, nonce := strconv.FormatInt(now.Unix(), 10), hex.EncodeToString(raw)
	req.Header.Set(hdrNodeID, nodeID)
	req.Header.Set(hdrNodeTime, ts)
	req.Header.Set(hdrNodeNonce, nonce)
	req.Header.Set(hdrNodeSig, nodeRPCSignature(key, req.Method, req.URL.Path, nodeID, ts, nonce, body))
	return nil
}

// nodeRPCVerifier checks signatures and remembers nonces for twice the skew,
// so a captured request cannot be replayed.
type nodeRPCVerifier struct {
	key func() []byte
	now func() time.Time

	mu   sync.Mutex
	seen map[string]time.Time
}

func newNodeRPCVerifier(key func() []byte) *nodeRPCVerifier {
	return &nodeRPCVerifier{key: key, now: time.Now, seen: map[string]time.Time{}}
}

// verify returns the calling node's id, or an error that says why the request
// is refused. It never reveals the expected signature.
func (v *nodeRPCVerifier) verify(r *http.Request, body []byte) (string, error) {
	key := v.key()
	if len(key) == 0 {
		return "", errors.New("node RPC key not initialised")
	}
	node, ts, nonce, sig := r.Header.Get(hdrNodeID), r.Header.Get(hdrNodeTime), r.Header.Get(hdrNodeNonce), r.Header.Get(hdrNodeSig)
	if node == "" || ts == "" || len(nonce) != 32 || sig == "" {
		return "", errors.New("missing node authentication headers")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", errors.New("bad timestamp")
	}
	now := v.now()
	if d := now.Sub(time.Unix(sec, 0)); d > nodeRPCSkew || d < -nodeRPCSkew {
		return "", errors.New("timestamp outside the allowed clock skew")
	}
	want := nodeRPCSignature(key, r.Method, r.URL.Path, node, ts, nonce, body)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return "", errors.New("bad signature")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for n, exp := range v.seen {
		if now.After(exp) {
			delete(v.seen, n)
		}
	}
	if _, dup := v.seen[nonce]; dup {
		return "", errors.New("replayed request")
	}
	v.seen[nonce] = now.Add(2 * nodeRPCSkew)
	return node, nil
}

// handleNodeRPC serves POST /api/node/rpc/{op} on the control node. requireAuth
// lets the prefix through; this handler is its only authentication.
func (s *Server) handleNodeRPC(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, nodeRPCMaxBody))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	node, err := s.nodeRPC.verify(r, body)
	if err != nil {
		log.Printf("[node-rpc] refused %s from %s: %v", r.URL.Path, r.RemoteAddr, err)
		writeErr(w, http.StatusUnauthorized, "node authentication failed")
		return
	}
	if s.m.traffic == nil {
		writeErr(w, http.StatusServiceUnavailable, "the control node has no traffic recording store")
		return
	}
	out, code, err := s.serveNodeRPC(r.Context(), r.PathValue("op"), body)
	if err != nil {
		if code >= 500 {
			log.Printf("[node-rpc] %s from node %s: %v", r.PathValue("op"), node, err)
		}
		writeJSON(w, code, nodeRPCError{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) serveNodeRPC(ctx context.Context, op string, body []byte) (any, int, error) {
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	switch op {
	case "tool":
		var in nodeToolReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		// Only the capture store's own tools: the allowlist is what the control
		// node would hand its own agents.
		for _, t := range s.m.traffic.Tools() {
			if t.Name() == in.Name {
				res, err := t.Call(ctx, in.Input, &actool.ToolContext{})
				if err != nil {
					return nil, http.StatusInternalServerError, err
				}
				res.Extra = nil // never relay the privileged channel across nodes
				return nodeToolReply{Result: res}, http.StatusOK, nil
			}
		}
		return nil, http.StatusNotFound, fmt.Errorf("no traffic tool %q", in.Name)
	case "fact":
		var in nodeFactReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		id, ok, err := s.m.traffic.ResponseContaining(in.Lines, in.FromTS, in.ToTS)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return nodeFactReply{ID: id, OK: ok}, http.StatusOK, nil
	case "exists":
		var in nodeExistsReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		ok, err := s.evidenceStore().TrafficExists(ctx, in.TrafficID)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		return nodeExistsReply{Exists: ok}, http.StatusOK, nil
	case "record":
		var in nodeRecordReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		out, err := s.evidenceStore().Record(ctx, in.Input, in.Refs)
		if err != nil {
			return nil, http.StatusUnprocessableEntity, err
		}
		return out, http.StatusOK, nil
	case "assess":
		var in nodeAssessReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		st, note := s.assessLocalFindingVerification(ctx, in.FindingID, in.POE, in.Quotes)
		return nodeAssessReply{Status: st, Note: note}, http.StatusOK, nil
	case "preview":
		var in bindingPreviewReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		out, err := previewBinding(ctx, s.evidenceStore(), in)
		if err != nil {
			return nil, http.StatusUnprocessableEntity, err
		}
		return out, http.StatusOK, nil
	case "bind":
		var in nodeBindReq
		if err := dec(&in); err != nil {
			return nil, http.StatusBadRequest, err
		}
		out, err := s.evidenceStore().Bind(ctx, in.FindingID, in.Refs)
		if err != nil {
			return nil, http.StatusUnprocessableEntity, err
		}
		return out, http.StatusOK, nil
	}
	return nil, http.StatusNotFound, fmt.Errorf("unknown node RPC %q", op)
}
