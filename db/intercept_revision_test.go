package db

import (
	"sync"
	"testing"
)

// restoreRevision puts the revision row back as the test found it.
func restoreRevision(t *testing.T, d *DB) {
	t.Helper()
	var prev string
	had := d.QueryRow(`SELECT value FROM settings WHERE key=$1`, InterceptRulesRevisionKey).Scan(&prev) == nil
	t.Cleanup(func() {
		if had {
			_, _ = d.Exec(`UPDATE settings SET value=$2 WHERE key=$1`, InterceptRulesRevisionKey, prev)
			return
		}
		_, _ = d.Exec(`DELETE FROM settings WHERE key=$1`, InterceptRulesRevisionKey)
	})
}

func TestInterceptRulesRevisionBumps(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	restoreRevision(t, d)

	if _, err := d.Exec(`DELETE FROM settings WHERE key=$1`, InterceptRulesRevisionKey); err != nil {
		t.Fatal(err)
	}
	if n, err := d.InterceptRulesRevision(); err != nil || n != 0 {
		t.Fatalf("unset revision = %d, %v; want 0, nil", n, err)
	}
	if n, err := d.BumpInterceptRulesRevision(); err != nil || n != 1 {
		t.Fatalf("first bump = %d, %v; want 1", n, err)
	}

	// Concurrent bumps (as from several processes) each get a distinct value.
	const workers = 16
	var wg sync.WaitGroup
	got := make(chan int64, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := d.BumpInterceptRulesRevision()
			if err != nil {
				t.Error(err)
				return
			}
			got <- n
		}()
	}
	wg.Wait()
	close(got)
	seen := map[int64]bool{}
	for n := range got {
		if seen[n] {
			t.Errorf("revision %d returned twice", n)
		}
		seen[n] = true
	}
	if n, err := d.InterceptRulesRevision(); err != nil || n != 1+workers {
		t.Fatalf("revision after %d concurrent bumps = %d, %v; want %d", workers, n, err, 1+workers)
	}
}

// A malformed stored value is an error on read and on bump: it is never read
// as a revision, and a bump never silently resets the counter.
func TestInterceptRulesRevisionMalformedIsError(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	restoreRevision(t, d)

	if err := d.SetSetting(InterceptRulesRevisionKey, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if n, err := d.InterceptRulesRevision(); err == nil {
		t.Fatalf("malformed revision read = %d, nil; want an error", n)
	}
	if n, err := d.BumpInterceptRulesRevision(); err == nil {
		t.Fatalf("malformed revision bump = %d, nil; want an error", n)
	}
}
