package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/db"
)

// Console user management (admin only: rbacPolicy requires admin for
// /api/users and below). Password hashes never leave the server. A user
// cannot demote, disable or delete themselves; the legacy ARTEX login is not
// a row here and always remains as break-glass admin.

// usernamePattern matches the CHECK constraint on artex_users.username.
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

// registerRBACRoutes mounts the user-management API. server.go calls it from
// Handler().
func (s *Server) registerRBACRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/users", s.listUsers)
	mux.HandleFunc("POST /api/users", s.createUser)
	mux.HandleFunc("PATCH /api/users/{id}", s.updateUser)
	mux.HandleFunc("DELETE /api/users/{id}", s.deleteUser)
}

// reservedActors are actor names the audit chain and the intercept history
// use for decisions no console user took.
var reservedActors = map[string]bool{"system": true, "anonymous": true, "human": true, "rule": true,
	"model": true, "target": true, "db": true, "admin": true}

// validateUsername returns "" or a reason ready to show the user.
func validateUsername(u string) string {
	if u == strings.ToLower(legacyUsername) {
		return "username \"artex\" is reserved for the bootstrap admin"
	}
	if reservedActors[u] {
		return fmt.Sprintf("username %q is reserved: the audit trail uses it for automatic actors", u)
	}
	if !usernamePattern.MatchString(u) {
		return "username must be 2-64 characters of a-z, 0-9, '.', '_' or '-', starting with a letter or digit"
	}
	return ""
}

// actorName is who a user-management write is recorded for.
func actorName(r *http.Request) string {
	if p, ok := principalFrom(r.Context()); ok {
		return p.Username
	}
	return ""
}

func writeUserErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrUserNotFound):
		writeErr(w, 404, "user not found")
	case errors.Is(err, db.ErrUserExists):
		writeErr(w, 409, "username already exists")
	default:
		writeErr(w, 500, "user store error: "+err.Error())
	}
}

// GET /api/users
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	users, err := pg.ListUsers()
	if err != nil {
		writeUserErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"users": users, "roles": db.UserRoles})
}

// POST /api/users {username, password, role}
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if msg := validateUsername(req.Username); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if !db.ValidUserRole(req.Role) {
		writeErr(w, 400, "role must be one of "+strings.Join(db.UserRoles, ", "))
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "Password encryption failed")
		return
	}
	u, err := pg.CreateUser(actorName(r), req.Username, string(hash), req.Role)
	if err != nil {
		writeUserErr(w, err)
		return
	}
	writeJSON(w, 201, u)
}

// PATCH /api/users/{id} {role?, disabled?, password?}. Any change bumps the
// user's version, which ends its sessions.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid user id")
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
		Password *string `json:"password"`
	}
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	if req.Role != nil && !db.ValidUserRole(*req.Role) {
		writeErr(w, 400, "role must be one of "+strings.Join(db.UserRoles, ", "))
		return
	}
	if me, ok := principalFrom(r.Context()); ok && me.UserID == id {
		if (req.Role != nil && *req.Role != db.RoleAdmin) || (req.Disabled != nil && *req.Disabled) {
			writeErr(w, 403, "you cannot demote or disable your own account")
			return
		}
	}
	patch := db.UserPatch{Role: req.Role, Disabled: req.Disabled}
	if req.Password != nil {
		if msg := validatePassword(*req.Password); msg != "" {
			writeErr(w, 400, msg)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeErr(w, 500, "Password encryption failed")
			return
		}
		h := string(hash)
		patch.PasswordHash = &h
	}
	u, err := pg.UpdateUser(actorName(r), id, patch)
	if err != nil {
		writeUserErr(w, err)
		return
	}
	writeJSON(w, 200, u)
}

// DELETE /api/users/{id}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "invalid user id")
		return
	}
	if me, ok := principalFrom(r.Context()); ok && me.UserID == id {
		writeErr(w, 403, "you cannot delete your own account")
		return
	}
	if err := pg.DeleteUser(actorName(r), id); err != nil {
		writeUserErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
