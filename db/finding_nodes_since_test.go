package db

import "testing"

// FindingNodesSince pages an exploration's findings after a cursor and names
// the intent each was found on: the yields edge when the finding named one,
// else the intent its writer holds the claim on (a worker that reported
// without intent_id).
func TestFindingNodesSince(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })
	tk, err := d.CreateTask("finding wake-up relay", "goal", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.DeleteTask(tk.ID) })
	es := d.Exploration(tk.ExplorationID)
	start, err := es.LatestFindingNodeID()
	if err != nil {
		t.Fatal(err)
	}
	const remote = "node-b/work#1"
	claimed := addClaimIntent(t, es)
	claimIntentToken(t, es, claimed, remote)
	named := addClaimIntent(t, es)

	record := func(intent int64, worker, summary string) int64 {
		t.Helper()
		in := RecordFindingInput{TaskID: tk.ID, ExplorationID: tk.ExplorationID, IntentID: intent, VulnClass: "IDOR", Severity: "high", Summary: summary, Worker: worker}
		rec, err := es.RecordFinding(t.Context(), in)
		if err != nil {
			t.Fatal(err)
		}
		return rec.NodeID
	}
	a := record(0, remote, "via claim")
	b := record(named, remote, "via edge")
	c := record(named, "work#2", "local")

	got, last, err := es.FindingNodesSince(start)
	if err != nil {
		t.Fatal(err)
	}
	want := []FindingNode{{a, remote, claimed, "via claim"}, {b, remote, named, "via edge"}, {c, "work#2", named, "local"}}
	if len(got) != len(want) || last != c {
		t.Fatalf("FindingNodesSince(%d) = %+v, last %d; want %+v, last %d", start, got, last, want, c)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("finding %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if more, l, err := es.FindingNodesSince(last); err != nil || len(more) != 0 || l != last {
		t.Fatalf("after the last finding: %+v, %d, %v; want none, cursor kept", more, l, err)
	}
	if latest, err := es.LatestFindingNodeID(); err != nil || latest != c {
		t.Fatalf("LatestFindingNodeID = %d, %v; want %d", latest, err, c)
	}
}
