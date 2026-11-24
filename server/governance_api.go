package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
)

// Engagement governance endpoints (E5): the per-task authorization artifact
// and the hash-chained audit log that extends the intercept history. The
// artifact is enforced only by the target gate (intercept/authorization.go);
// these handlers store and show it. Who may call them is decided by the role
// policy in requireAuth (writes are admin-only).

func (s *Server) registerGovernanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks/{id}/authorization", s.getTaskAuthorization)
	mux.HandleFunc("POST /api/tasks/{id}/authorization", s.putTaskAuthorization)
	mux.HandleFunc("POST /api/tasks/{id}/authorization/revoke", s.revokeTaskAuthorization)
	mux.HandleFunc("GET /api/intercept/history/chain", s.listAuditChain)
	mux.HandleFunc("GET /api/intercept/history/chain/verify", s.verifyAuditChain)
}

// authorizationView is the GET answer: the version in force (or the newest),
// whether it authorizes action now, the history, and the classes an artifact may name.
type authorizationView struct {
	Current    *db.EngagementAuthorization   `json:"current"`
	InForce    bool                          `json:"in_force"`
	Versions   []*db.EngagementAuthorization `json:"versions"`
	Techniques []string                      `json:"technique_classes"`
	ServerTime time.Time                     `json:"server_time"`
}

func (s *Server) authorizationTaskID(w http.ResponseWriter, r *http.Request) (*db.DB, int64, bool) {
	pg := s.pg(w)
	if pg == nil {
		return nil, 0, false
	}
	id, ok := s.taskCredentialTaskID(w, r)
	return pg, id, ok
}

// GET /api/tasks/{id}/authorization
func (s *Server) getTaskAuthorization(w http.ResponseWriter, r *http.Request) {
	pg, id, ok := s.authorizationTaskID(w, r)
	if !ok {
		return
	}
	versions, err := pg.ListAuthorizations(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	now := time.Now().UTC()
	v := authorizationView{Versions: versions, Techniques: intercept.TechniqueClasses(), ServerTime: now}
	if len(versions) > 0 {
		v.Current = versions[0]
		v.InForce = v.Current.InForce(now)
	}
	writeJSON(w, 200, v)
}

// POST /api/tasks/{id}/authorization — stores a new version of the artifact.
func (s *Server) putTaskAuthorization(w http.ResponseWriter, r *http.Request) {
	pg, id, ok := s.authorizationTaskID(w, r)
	if !ok {
		return
	}
	var in db.AuthorizationInput
	if err := decode(w, r, &in); err != nil {
		writeErr(w, 400, "invalid request body: "+err.Error())
		return
	}
	in, err := in.Normalize(intercept.TechniqueClasses())
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.storeAuthorization(w, r, pg, id, in, false)
}

// POST /api/tasks/{id}/authorization/revoke — a new version with the same
// terms that authorizes nothing. Optional body {"note": "..."}.
func (s *Server) revokeTaskAuthorization(w http.ResponseWriter, r *http.Request) {
	pg, id, ok := s.authorizationTaskID(w, r)
	if !ok {
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := decode(w, r, &req); err != nil {
			writeErr(w, 400, "invalid request body: "+err.Error())
			return
		}
	}
	cur, err := pg.CurrentAuthorization(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if cur == nil {
		writeErr(w, 404, "the task has no authorization to revoke")
		return
	}
	if cur.Revoked {
		writeErr(w, 409, "the authorization is already revoked")
		return
	}
	in := cur.Input()
	if req.Note != "" {
		in.Note = req.Note
	}
	s.storeAuthorization(w, r, pg, id, in, true)
}

func (s *Server) storeAuthorization(w http.ResponseWriter, r *http.Request, pg *db.DB, id int64, in db.AuthorizationInput, revoked bool) {
	actor := s.approver(r)
	if actor == "" {
		writeErr(w, 401, "Unauthorized")
		return
	}
	a, err := pg.AddAuthorization(id, in, revoked, actor)
	if errors.Is(err, db.ErrAuthorizationNoTask) {
		writeErr(w, 404, "task not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

// GET /api/intercept/history/chain?after_seq=&kind=&subject=&limit= — chain
// entries in order. kind may be a prefix ending in "." (row.).
func (s *Server) listAuditChain(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	q := r.URL.Query()
	f := db.AuditFilter{Kind: q.Get("kind"), Subject: q.Get("subject")}
	if v := q.Get("after_seq"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeErr(w, 400, "after_seq must be a non-negative integer")
			return
		}
		f.AfterSeq = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeErr(w, 400, "limit must be a positive integer")
			return
		}
		f.Limit = n
	}
	entries, err := pg.ListAudit(f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries})
}

// GET /api/intercept/history/chain/verify — recomputes the chain and compares
// the intercept history rows with it. 200 either way; ok says the result.
func (s *Server) verifyAuditChain(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	v, err := pg.VerifyAudit(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}
