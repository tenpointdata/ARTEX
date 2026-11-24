package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/traffic"
	actool "github.com/Autumn-27/norma/tool"
)

// controlRPC is a worker node's client of the control node's node RPC
// (node_rpc.go). It implements, for the agents of this node, what the capture
// store and the evidence store do on the control node.
type controlRPC struct {
	base   string // control API, scheme://host:port
	nodeID string
	key    func() []byte
	hc     *http.Client
}

// defaultControlAPIPort is the control node's API port (-addr :8787), used when
// ARTEX_CONTROL_API is not set.
const defaultControlAPIPort = "8787"

// controlAPIURL is ARTEX_CONTROL_API, or, when it is empty, the control proxy's
// host on the API port: https:// when a control CA is pinned, else http://.
func controlAPIURL(explicit, controlProxy string, pinnedCA bool) (string, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		u, err := url.Parse(controlProxy)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("cannot derive the control API from ARTEX_CONTROL_PROXY; set ARTEX_CONTROL_API")
		}
		scheme := "http"
		if pinnedCA {
			scheme = "https"
		}
		raw = scheme + "://" + net.JoinHostPort(u.Hostname(), defaultControlAPIPort)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return "", fmt.Errorf("ARTEX_CONTROL_API %q must be http(s)://host:port with no credentials or path", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// newControlRPC calls the control API at base with hc (controlHTTPClient); nil
// is a plain client.
func newControlRPC(base, nodeID string, key func() []byte, hc *http.Client) *controlRPC {
	if hc == nil {
		hc = &http.Client{Timeout: controlRPCTimeout}
	}
	return &controlRPC{base: base, nodeID: nodeID, key: key, hc: hc}
}

// call posts in to op and decodes the reply into out.
func (c *controlRPC) call(ctx context.Context, op string, in, out any) error {
	key := c.key()
	if len(key) == 0 {
		return errors.New("node RPC key not initialised")
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+nodeRPCPrefix+op, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := signNodeRPC(req, key, c.nodeID, body, time.Now()); err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("control node unreachable for %s: %w", op, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, nodeRPCMaxReply+1))
	if err != nil {
		return err
	}
	if len(raw) > nodeRPCMaxReply {
		return fmt.Errorf("control node %s reply too large", op)
	}
	want := nodeRPCReplySignature(key, req.Header.Get(hdrNodeNonce), resp.StatusCode, raw)
	if !hmac.Equal([]byte(want), []byte(resp.Header.Get(hdrReplySig))) {
		return fmt.Errorf("control node %s: reply not signed by this deployment (HTTP %d)", op, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		var e nodeRPCError
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("control node %s: HTTP %d", op, resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

// ResponseContaining is agent.FactTrafficLookup through the control node. It
// has no context of its own, so it is bounded here.
func (c *controlRPC) ResponseContaining(lines []string, fromTS, toTS int64) (string, bool, error) {
	if len(lines) == 0 {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out nodeFactReply
	if err := c.call(ctx, "fact", nodeFactReq{Lines: lines, FromTS: fromTS, ToTS: toTS}, &out); err != nil {
		return "", false, err
	}
	return out.ID, out.OK, nil
}

// Tools are the capture store's agent tools, run on the control node. The
// names, descriptions and schemas are traffic.Tools' own (built without a store;
// only their Run touches it), so a worker node's agents see the same tools.
func (c *controlRPC) Tools() []actool.CoreTool {
	local := (*traffic.Traffic)(nil).Tools()
	out := make([]actool.CoreTool, 0, len(local))
	for _, t := range local {
		out = append(out, remoteTool{CoreTool: t, rpc: c})
	}
	return out
}

// remoteTool keeps a traffic tool's declaration and permissions and replaces
// only its execution.
type remoteTool struct {
	actool.CoreTool
	rpc *controlRPC
}

func (t remoteTool) Call(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
	var out nodeToolReply
	if err := t.rpc.call(ctx, "tool", nodeToolReq{Name: t.Name(), Input: in}, &out); err != nil {
		return actool.Errorf(err.Error()), nil
	}
	out.Result.Extra = nil
	return out.Result, nil
}

// remoteEvidence is the worker node's finding evidence store: a finding with
// traffic refs, a bind and an existence check run on the control node, which
// holds the capture. A finding without refs is recorded here, so a control node
// that is briefly unreachable does not lose a text-only finding.
type remoteEvidence struct {
	local localEvidenceRecorder
	rpc   *controlRPC
}

// localEvidenceRecorder is the evidence.Store subset that needs no capture.
type localEvidenceRecorder interface {
	Record(context.Context, db.RecordFindingInput, []db.TrafficRef) (*db.RecordedFinding, error)
}

func (r remoteEvidence) Record(ctx context.Context, in db.RecordFindingInput, refs []db.TrafficRef) (*db.RecordedFinding, error) {
	if len(refs) == 0 {
		return r.local.Record(ctx, in, refs)
	}
	out := &db.RecordedFinding{}
	if err := r.rpc.call(ctx, "record", nodeRecordReq{Input: in, Refs: refs}, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r remoteEvidence) Bind(ctx context.Context, findingID int64, refs []db.TrafficRef) (*db.FindingTraffic, error) {
	out := &db.FindingTraffic{}
	if err := r.rpc.call(ctx, "bind", nodeBindReq{FindingID: findingID, Refs: refs}, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r remoteEvidence) Preview(ctx context.Context, in bindingPreviewReq) (*bindingPreview, error) {
	out := &bindingPreview{}
	if err := r.rpc.call(ctx, "preview", in, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r remoteEvidence) TrafficExists(ctx context.Context, trafficID string) (bool, error) {
	var out nodeExistsReply
	if err := r.rpc.call(ctx, "exists", nodeExistsReq{TrafficID: trafficID}, &out); err != nil {
		return false, err
	}
	return out.Exists, nil
}

// findingEvidence is what agents use to record findings and bind traffic: the
// evidence.Store on a node with the capture, remoteEvidence on a worker node.
type findingEvidence interface {
	agent.FindingRecorder
	agent.TrafficCaptureChecker
	Bind(context.Context, int64, []db.TrafficRef) (*db.FindingTraffic, error)
	Preview(context.Context, bindingPreviewReq) (*bindingPreview, error)
}
