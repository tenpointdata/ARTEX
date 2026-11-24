package intercept

import (
	"log"
	"time"
)

// The rule cache across processes.
//
// Several processes (the control node and its workers) share one database. A
// rule or enabled-tools change is written by one of them, usually the control
// node, which serves the API. The cache of every other process must follow, or
// a rule added or tightened there is not enforced here (fail-open). The
// mechanism is a counter in the settings table (db.InterceptRulesRevisionKey):
//
//   - Invalidate, which every rule or tool-config write calls after it commits,
//     clears this process's cache and bumps the counter.
//   - Every read of the cache (Match, MatchBuiltin, MatchPersistence,
//     IsToolEnabled) first compares the revision the cache was loaded at with
//     the stored one, and reloads when they differ. The revision is read
//     before the rules, so a load can only be labelled older than what it
//     holds, never newer: a concurrent change at worst costs one extra reload.
//   - A revision that cannot be read is a store failure: the cache is dropped
//     and the call is denied, as when the rules themselves cannot be read. The
//     stale cache is never served as authoritative.
//
// Window: a confirmed revision is trusted for rulesRevisionTTL, so a change
// made by another process is enforced here at most that long after it was
// committed. The process that made the change sees it on its next call.
// rulesMaxAge forces a full reload even when the revision did not move, which
// bounds the staleness left by a bump that failed after its write committed.

// rulesRevisionTTL is how long a confirmed revision is trusted before the next
// read checks it again. Zero checks on every read.
var rulesRevisionTTL = time.Second

// rulesMaxAge is how long a load is used, revision unchanged, before the next
// read reloads it.
var rulesMaxAge = 30 * time.Second

// Invalidate clears the in-memory rule cache and the enabled-tools cache and
// bumps the shared rules revision, so the next call of every process that
// shares the database reloads from it. Call it after any committed write to
// the rules or the tool config. An error means the bump failed: this process
// reloads, but the others see the change only after rulesMaxAge.
func (i *Interceptor) Invalidate() error {
	i.mu.Lock()
	i.dropLocked()
	i.mu.Unlock()
	if i.db == nil {
		return nil
	}
	if _, err := i.db.BumpInterceptRulesRevision(); err != nil {
		log.Printf("[intercept] the rules revision could not be bumped; other processes may enforce the previous rules for up to %s: %v", rulesMaxAge, err)
		return err
	}
	return nil
}

// current returns the rules and the enabled-tools set, reloading them when the
// stored revision moved. It returns an error, and nothing, when the revision or
// the rules cannot be read: the callers then fail closed.
func (i *Interceptor) current() ([]compiledRule, map[string]bool, error) {
	i.mu.RLock()
	if i.freshLocked(time.Now()) {
		rules, tools := i.cached, i.enabledTools
		i.mu.RUnlock()
		return rules, tools, nil
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now()
	if i.freshLocked(now) {
		return i.cached, i.enabledTools, nil
	}
	if i.revision == nil {
		// Store-less interceptor (New(nil)): it holds what SetRulesForTest
		// installed; loadLocked reads the database it was built on.
		if err := i.loadLocked(); err != nil {
			return nil, nil, err
		}
		return i.cached, i.enabledTools, nil
	}
	rev, err := i.revision()
	if err != nil {
		i.dropLocked()
		i.storeFailedLocked(err)
		return nil, nil, err
	}
	if i.loaded && rev == i.cachedRev && now.Sub(i.loadedAt) < rulesMaxAge {
		i.checkedAt = now
		return i.cached, i.enabledTools, nil
	}
	i.dropLocked()
	if err := i.loadLocked(); err != nil {
		return nil, nil, err
	}
	i.cachedRev, i.checkedAt, i.loadedAt = rev, now, now
	return i.cached, i.enabledTools, nil
}

// freshLocked reports whether the cache may be served without asking the
// store. A store-less interceptor's cache is always fresh once installed.
func (i *Interceptor) freshLocked(now time.Time) bool {
	if !i.loaded {
		return false
	}
	if i.revision == nil {
		return true
	}
	return now.Sub(i.checkedAt) < rulesRevisionTTL && now.Sub(i.loadedAt) < rulesMaxAge
}

// dropLocked empties the cache; the next read reloads it.
func (i *Interceptor) dropLocked() {
	i.cached, i.enabledTools, i.loaded = nil, nil, false
}

// storeFailedLocked logs a store failure once per outage.
func (i *Interceptor) storeFailedLocked(err error) {
	if i.loadFailed {
		return
	}
	i.loadFailed = true
	log.Printf("[intercept] the rule store cannot be read; every tool call is denied until it can: %v", err)
}

// installForTest installs a rule set as the loaded one (SetRulesForTest). On an
// interceptor with a store the set is labelled with the current revision, so a
// later change of the stored rules replaces it; when the revision cannot be
// read the set is labelled unknown and the next read reloads.
func (i *Interceptor) installForTest(rules []compiledRule, tools map[string]bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cached, i.enabledTools, i.loaded = rules, tools, true
	if i.revision == nil {
		return
	}
	now := time.Now()
	rev, err := i.revision()
	if err != nil {
		i.dropLocked()
		return
	}
	i.cachedRev, i.checkedAt, i.loadedAt = rev, now, now
}
