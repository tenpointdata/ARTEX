package server

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Per-run executor identities. With a uid range configured (-exec-uid-range), each
// task and each chat session gets a uid of its own from the range while it is live,
// and its run directories belong to that uid alone: tasks/<id>/main,
// tasks/<id>/i<N> and sessions/<id> become <uid>:<uid> 2770 (the gid is the same
// number). An agent shell of task A then cannot write, list or enter a run
// directory of task B, even when it knows the path. Without a range every agent
// process shares -exec-uid, the single-uid layout of execpriv_unix.go.
//
// The owner of a run directory is the identity of the processes that work in it:
// the bash shim, custom tools and the file tools read it from the directory
// (execIdentityIn), so no channel from the server to a shim is needed. Only root
// can change the owner, and only the server creates run directories.

// ExecUIDRange is an inclusive range of uids handed to tasks and chat sessions.
// The zero value is no range.
type ExecUIDRange struct{ Lo, Hi uint32 }

// Enabled reports whether a range is configured.
func (r ExecUIDRange) Enabled() bool { return r.Lo != 0 }

// Contains reports whether id is in the range.
func (r ExecUIDRange) Contains(id uint32) bool { return r.Enabled() && id >= r.Lo && id <= r.Hi }

// String renders the range the way -exec-uid-range and iptables --uid-owner take it.
func (r ExecUIDRange) String() string {
	if !r.Enabled() {
		return ""
	}
	return execIDString(r.Lo) + "-" + execIDString(r.Hi)
}

// ParseExecUIDRange parses "LO-HI". An empty string is no range.
func ParseExecUIDRange(s string) (ExecUIDRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ExecUIDRange{}, nil
	}
	lo, hi, ok := strings.Cut(s, "-")
	l, errL := strconv.ParseUint(strings.TrimSpace(lo), 10, 31)
	h, errH := strconv.ParseUint(strings.TrimSpace(hi), 10, 31)
	if !ok || errL != nil || errH != nil || l == 0 || h < l {
		return ExecUIDRange{}, fmt.Errorf("-exec-uid-range %q must be LO-HI with 0 < LO <= HI < 2^31", s)
	}
	return ExecUIDRange{Lo: uint32(l), Hi: uint32(h)}, nil
}

// WithUIDRange returns p with per-run uids from r. The range must leave out the
// executor uid and gid and the MCP uid: a task gid equal to the executor gid would
// open its run directories to every agent shell, and a task uid equal to another
// identity would share its processes. A disabled p ignores the range.
func (p ExecPrivilege) WithUIDRange(r ExecUIDRange) (ExecPrivilege, error) {
	if !p.Enabled || !r.Enabled() {
		return p, nil
	}
	for _, c := range []struct {
		name string
		id   uint32
	}{{"-exec-uid", p.UID}, {"-exec-gid", p.GID}, {"the MCP uid", p.mcpUID()}} {
		if r.Contains(c.id) {
			return ExecPrivilege{}, fmt.Errorf("-exec-uid-range %s must not contain %s %d", r, c.name, c.id)
		}
	}
	p.Range = r
	return p, nil
}

// execUIDPool hands out uids of a range to run keys ("tasks/<id>", "sessions/<id>").
// Allocation is round-robin from the last uid handed out, so a released uid is
// reused only after the whole range has gone round.
type execUIDPool struct {
	mu    sync.Mutex
	r     ExecUIDRange
	next  uint32
	byKey map[string]uint32
	byUID map[uint32]string
}

func newExecUIDPool(r ExecUIDRange) *execUIDPool {
	return &execUIDPool{r: r, next: r.Lo, byKey: map[string]uint32{}, byUID: map[uint32]string{}}
}

// acquire returns the uid of key, allocating one when it has none; fresh reports
// an allocation. It fails when every uid of the range is in use.
func (p *execUIDPool) acquire(key string) (uid uint32, fresh bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if uid, ok := p.byKey[key]; ok {
		return uid, false, nil
	}
	size := uint64(p.r.Hi-p.r.Lo) + 1
	for i := uint64(0); i < size; i++ {
		c := p.next
		if p.next == p.r.Hi {
			p.next = p.r.Lo
		} else {
			p.next++
		}
		if _, used := p.byUID[c]; !used {
			p.byKey[key], p.byUID[c] = c, key
			return c, true, nil
		}
	}
	return 0, false, fmt.Errorf("every executor uid of %s is in use: no uid for %s", p.r, key)
}

// adopt records uid as key's (a run directory found on disk at start-up). It
// reports false when uid is outside the range or already another key's.
func (p *execUIDPool) adopt(key string, uid uint32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.r.Contains(uid) {
		return false
	}
	if k, used := p.byUID[uid]; used {
		return k == key
	}
	if old, ok := p.byKey[key]; ok {
		return old == uid
	}
	p.byKey[key], p.byUID[uid] = uid, key
	if uid >= p.next {
		p.next = uid
		if p.next == p.r.Hi {
			p.next = p.r.Lo
		} else {
			p.next++
		}
	}
	return true
}

// release frees key's uid and returns it.
func (p *execUIDPool) release(key string) (uint32, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	uid, ok := p.byKey[key]
	if ok {
		delete(p.byKey, key)
		delete(p.byUID, uid)
	}
	return uid, ok
}

// runKeyOf returns the run key of a run directory leaf below base: "tasks/<id>" for
// tasks/<id>/<leaf>, "sessions/<id>" for sessions/<id>.
func runKeyOf(base, leaf string) (string, error) {
	rel, err := filepath.Rel(base, leaf)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%s is not below %s", leaf, base)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	switch {
	case len(parts) == 3 && parts[0] == "tasks" && isDigits(parts[1]):
		return "tasks/" + parts[1], nil
	case len(parts) == 2 && parts[0] == "sessions" && parts[1] != "":
		return "sessions/" + parts[1], nil
	}
	return "", fmt.Errorf("%s is not a run directory", leaf)
}

// isDigits reports whether s is a non-empty string of ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
