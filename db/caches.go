package db

import (
	"time"

	"github.com/Autumn-27/artex/memo"
)

// Read-through caches for hot per-call lookups whose writes go through this
// package, so the writer invalidates them (see the memo package). The TTL is
// the backstop for a change made outside this process (another instance, psql):
// such a change is seen within it.
//
// A DB built by Open has them; one built as &DB{DB: sqlDB} (tests, tools) has
// none and reads the database every time.
const (
	// authorizationCacheTTL bounds how long the target gate serves a task's
	// artifact without rereading it. AddAuthorization invalidates at once, so
	// a revocation through the API applies to the very next call.
	authorizationCacheTTL = 2 * time.Second
	// userCacheTTL bounds how long an open SSE stream re-check serves a
	// console user's row. UpdateUser, DeleteUser and ChangeOwnPassword
	// invalidate at once.
	userCacheTTL = 2 * time.Second
)

type dbCaches struct {
	authz memo.Cache[int64, *EngagementAuthorization]
	users memo.Cache[int64, User]
}

func newDBCaches() *dbCaches {
	c := &dbCaches{}
	c.authz.TTL = authorizationCacheTTL
	c.users.TTL = userCacheTTL
	return c
}

// CachedAuthorization is CurrentAuthorization through the per-task cache. It
// is for the target gate, which reads the artifact on every tool call and
// dial; the returned value is shared and must not be modified.
func (d *DB) CachedAuthorization(taskID int64) (*EngagementAuthorization, error) {
	if d.caches == nil {
		return d.CurrentAuthorization(taskID)
	}
	return d.caches.authz.Get(taskID, func() (*EngagementAuthorization, error) {
		return d.CurrentAuthorization(taskID)
	})
}

// CachedUser is GetUser through the user cache, for re-checking a session
// many times (each event of an SSE stream). A request's first check uses GetUser.
func (d *DB) CachedUser(id int64) (User, error) {
	if d.caches == nil {
		return d.GetUser(id)
	}
	return d.caches.users.Get(id, func() (User, error) { return d.GetUser(id) })
}

func (d *DB) invalidateAuthorization(taskID int64) {
	if d.caches != nil {
		d.caches.authz.Invalidate(taskID)
	}
}

func (d *DB) invalidateUser(id int64) {
	if d.caches != nil {
		d.caches.users.Invalidate(id)
	}
}
