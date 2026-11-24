package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// Capture-proxy scope tokens minted by worker nodes (migration 0023). A worker
// node's agents reach the control node's recording proxy with a proxy URL
// carrying a scope token (egress.ScopedProxyURL); the control node resolves the
// token to its task here and applies that task's TargetScope to the dial, as it
// does for a token it minted itself (traffic/scope.go). Only the SHA-256 of a
// token is stored: a database reader learns which tasks have tokens, never a
// credential that names one. A token is not an authorization (it can only add
// a task's rules to the global ones), but keeping it out of the database keeps
// a leaked row from widening the set of clients that may use the proxy.

// errEmptyProxyScopeToken refuses a token that cannot name a scope.
var errEmptyProxyScopeToken = errors.New("proxy scope token is empty")

// proxyScopeTokenHash is the stored form of token.
func proxyScopeTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// RegisterProxyScopeToken records token as a scope token of task taskID, minted
// by node nodeID. Re-registering the same token for the same task is a no-op;
// a token already registered for another task is an error (a collision of 128
// random bits, or a caller bug), so a token never silently changes scope.
func (d *DB) RegisterProxyScopeToken(taskID int64, token, nodeID string) error {
	if strings.TrimSpace(token) == "" {
		return errEmptyProxyScopeToken
	}
	if taskID <= 0 {
		return errors.New("proxy scope token: invalid task id")
	}
	var owner int64
	err := d.QueryRow(`
INSERT INTO proxy_scope_tokens(token_sha256, task_id, node_id) VALUES ($1, $2, $3)
ON CONFLICT (token_sha256) DO UPDATE SET token_sha256 = EXCLUDED.token_sha256
RETURNING task_id`, proxyScopeTokenHash(token), taskID, nodeID).Scan(&owner)
	if err != nil {
		return err
	}
	if owner != taskID {
		return errors.New("proxy scope token is already registered for another task")
	}
	return nil
}

// LookupProxyScopeToken returns the task token is registered for. ok is false
// when no row names it (never registered, or forgotten with its task); err is
// set only when the lookup itself failed, which a caller must treat as a
// refusal (fail closed), never as "no scope".
func (d *DB) LookupProxyScopeToken(token string) (taskID int64, ok bool, err error) {
	if token == "" {
		return 0, false, nil
	}
	err = d.QueryRow(`SELECT task_id FROM proxy_scope_tokens WHERE token_sha256 = $1`,
		proxyScopeTokenHash(token)).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return taskID, true, nil
}

// ForgetProxyScopeTokens drops every scope token of task taskID, whichever node
// minted it. The control node calls it when the task's scope ends, before it
// closes the proxy connections bound to them (traffic.ForgetScope), so a client
// that reconnects with a forgotten token is refused.
func (d *DB) ForgetProxyScopeTokens(taskID int64) error {
	_, err := d.Exec(`DELETE FROM proxy_scope_tokens WHERE task_id = $1`, taskID)
	return err
}

// ForgetNodeProxyScopeTokens drops the tokens of task taskID that node nodeID
// minted, for a worker node that stops running the task while others go on.
func (d *DB) ForgetNodeProxyScopeTokens(taskID int64, nodeID string) error {
	_, err := d.Exec(`DELETE FROM proxy_scope_tokens WHERE task_id = $1 AND node_id = $2`, taskID, nodeID)
	return err
}
