package db

import (
	"testing"
	"time"
)

// TestNodeRegistryHidesAndPrunesStaleRows: a live row and a recently dead row
// are listed; a row silent for longer than NodeRetention (a pod replaced by a
// Kubernetes rollout) is hidden by ListNodes and deleted by PruneNodes.
func TestNodeRegistryHidesAndPrunesStaleRows(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	ids := map[string]time.Duration{
		"prune-test-live":   0,
		"prune-test-recent": 2 * NodeTTL,
		"prune-test-stale":  NodeRetention + time.Minute,
	}
	for id, age := range ids {
		if err := d.HeartbeatNode(WorkerNode{NodeID: id, Role: "worker", Host: "h", Workers: 1}, true); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec(`UPDATE worker_nodes SET last_seen_at = now() - $2 * INTERVAL '1 second' WHERE node_id=$1`,
			id, age.Seconds()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = d.RemoveNode(id) })
	}
	listed := func() map[string]bool {
		t.Helper()
		nodes, err := d.ListNodes()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, n := range nodes {
			if _, ok := ids[n.NodeID]; ok {
				out[n.NodeID] = n.Live
			}
		}
		return out
	}
	want := map[string]bool{"prune-test-live": true, "prune-test-recent": false}
	if got := listed(); len(got) != 2 || got["prune-test-live"] != true || got["prune-test-recent"] != false {
		t.Fatalf("listed = %v, want %v (stale row hidden)", got, want)
	}
	gone, err := d.PruneNodes()
	if err != nil {
		t.Fatal(err)
	}
	if gone < 1 {
		t.Fatalf("PruneNodes removed %d rows, want the stale one", gone)
	}
	var n int
	if err := d.QueryRow(`SELECT count(*) FROM worker_nodes WHERE node_id = ANY($1)`,
		[]string{"prune-test-live", "prune-test-recent", "prune-test-stale"}).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("%d rows remain, want live and recently dead only", n)
	}
}
