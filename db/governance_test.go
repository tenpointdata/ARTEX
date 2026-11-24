package db

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

var knownTestTechniques = []string{"recon", "scanning", "exploitation"}

func TestAuthorizationInputNormalize(t *testing.T) {
	now := time.Now()
	in := AuthorizationInput{
		Scope:      []AuthScopeEntry{{"domain", "*.Example.COM."}, {"cidr", "10.0.0.7/24"}, {"ip", "::ffff:192.0.2.1"}, {"domain", "example.com"}},
		Techniques: []string{" Scanning", "recon", "scanning"},
		NotBefore:  now, NotAfter: now.Add(time.Hour),
	}
	out, err := in.Normalize(knownTestTechniques)
	if err != nil {
		t.Fatal(err)
	}
	want := []AuthScopeEntry{{"cidr", "10.0.0.0/24"}, {"domain", "example.com"}, {"ip", "192.0.2.1"}}
	if len(out.Scope) != len(want) {
		t.Fatalf("scope = %+v", out.Scope)
	}
	for i := range want {
		if out.Scope[i] != want[i] {
			t.Errorf("scope[%d] = %+v, want %+v", i, out.Scope[i], want[i])
		}
	}
	if strings.Join(out.Techniques, ",") != "recon,scanning" {
		t.Errorf("techniques = %v", out.Techniques)
	}
	for name, bad := range map[string]AuthorizationInput{
		"no window":      {},
		"reversed":       {NotBefore: now, NotAfter: now.Add(-time.Second)},
		"unknown class":  {NotBefore: now, NotAfter: now.Add(time.Hour), Techniques: []string{"magic"}},
		"bad kind":       {NotBefore: now, NotAfter: now.Add(time.Hour), Scope: []AuthScopeEntry{{"url", "x"}}},
		"bad cidr":       {NotBefore: now, NotAfter: now.Add(time.Hour), Scope: []AuthScopeEntry{{"cidr", "10.0.0.0/99"}}},
		"url as a host":  {NotBefore: now, NotAfter: now.Add(time.Hour), Scope: []AuthScopeEntry{{"host", "https://x"}}},
		"inner wildcard": {NotBefore: now, NotAfter: now.Add(time.Hour), Scope: []AuthScopeEntry{{"domain", "a.*.com"}}},
	} {
		if _, err := bad.Normalize(knownTestTechniques); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAuthorizationCoversHost(t *testing.T) {
	a := &EngagementAuthorization{Scope: []AuthScopeEntry{
		{"domain", "example.com"}, {"host", "api.other.test"}, {"cidr", "203.0.113.0/24"}, {"ip", "2001:db8::1"},
	}}
	for _, tc := range []struct {
		kind, host string
		want       bool
	}{
		{"domain", "example.com", true}, {"domain", "a.b.example.com", true}, {"domain", "EXAMPLE.com.", true},
		{"domain", "badexample.com", false}, {"domain", "api.other.test", true}, {"domain", "x.api.other.test", false},
		{"ip", "203.0.113.200", true}, {"ip", "203.0.114.1", false}, {"ip", "2001:db8::1", true}, {"ip", "2001:db8::2", false},
		{"cidr", "203.0.113.128/25", true}, {"cidr", "203.0.112.0/23", false}, {"ip", "::ffff:203.0.113.5", true},
		{"domain", "203.0.113.5.nip.io", false},
	} {
		if got := a.CoversHost(tc.kind, tc.host); got != tc.want {
			t.Errorf("CoversHost(%s, %s) = %v, want %v", tc.kind, tc.host, got, tc.want)
		}
	}
	if !(&EngagementAuthorization{}).CoversHost("domain", "anything.test") {
		t.Error("an empty scope must cover every target")
	}
}

func TestChainHashMatchesSQL(t *testing.T) {
	// A fixed vector, so a change to either side shows up without a database.
	got := chainHash(1, GenesisHash, "2026-11-24T00:00:00.000000Z", "ARTEX", "auth.login", "", `{"a":1}`)
	if len(got) != 64 {
		t.Fatalf("hash %q", got)
	}
	if chainHash(1, GenesisHash, "2026-11-24T00:00:00.000000Z", "ARTEX", "auth.login", "", `{"a":2}`) == got {
		t.Fatal("payload is not covered by the hash")
	}
	// Moving a byte across a field boundary changes the hash.
	if chainHash(1, GenesisHash, "t", "ab", "c", "", "") == chainHash(1, GenesisHash, "t", "a", "bc", "", "") {
		t.Fatal("fields are not delimited")
	}
}

func TestAuditChainPG(t *testing.T) {
	d := openTargetDB(t)
	ctx := context.Background()
	task := targetTask(t, d, TaskCreateOptions{})

	// Application events and trigger entries form one chain that verifies.
	seq, err := d.AppendAudit("tester", AuditLoginOK, "tester", map[string]string{"ip": "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	in, err := AuthorizationInput{Reference: "SOW-7", Scope: []AuthScopeEntry{{"domain", "example.com"}},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}.Normalize(knownTestTechniques)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := d.AddAuthorization(task.ID, in, false, "admin-a")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := d.AddAuthorization(task.ID, v1.Input(), true, "admin-b")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v2.Version != 2 || !v2.Revoked || v2.Reference != "SOW-7" {
		t.Fatalf("versions: %+v %+v", v1, v2)
	}
	cur, err := d.CurrentAuthorization(task.ID)
	if err != nil || cur == nil || cur.Version != 2 {
		t.Fatalf("current = %+v, %v", cur, err)
	}
	if _, err := d.Exec(`UPDATE engagement_authorizations SET revoked=false WHERE id=$1`, v2.ID); err == nil {
		t.Fatal("an artifact version was updated in place")
	}
	if _, err := d.AddAuthorization(-1, in, false, "admin-a"); err != ErrAuthorizationNoTask {
		t.Fatalf("missing task: %v", err)
	}

	// The artifact entry carries the actor set in its transaction.
	entries, err := d.ListAudit(AuditFilter{AfterSeq: seq, Kind: "row.engagement_authorizations", Subject: strconv.FormatInt(v2.ID, 10)})
	if err != nil || len(entries) != 1 || entries[0].Actor != "admin-b" {
		t.Fatalf("artifact entries = %+v, %v", entries, err)
	}

	// An intercept row is chained on insert and on decision.
	taskRef := strconv.FormatInt(task.ID, 10)
	id, err := d.CreateInterceptPending(0, 0, taskRef, "worker", "Bash", []byte(`{"command":"id"}`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ResolveInterceptBy(id, "allowed", "allow", "ok", "human", "reviewer-x", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	rows, err := d.ListAudit(AuditFilter{AfterSeq: seq, Kind: "row.intercept_pending", Subject: strconv.FormatInt(id, 10)})
	if err != nil || len(rows) != 2 {
		t.Fatalf("intercept entries = %+v, %v", rows, err)
	}
	var last struct {
		Op  string         `json:"op"`
		Row map[string]any `json:"row"`
	}
	if err := json.Unmarshal(rows[1].Payload, &last); err != nil || last.Op != "UPDATE" || last.Row["status"] != "allowed" {
		t.Fatalf("decision entry = %s", rows[1].Payload)
	}
	if _, ok := last.Row["tool_input"]; ok {
		t.Fatal("the entry carries the raw tool input instead of its digest")
	}

	res, err := d.VerifyAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Entries < 5 || res.HeadSeq != res.Entries {
		t.Fatalf("verify = %+v", res)
	}
	if _, err := d.Exec(`DELETE FROM audit_chain WHERE seq=$1`, seq); err == nil {
		t.Fatal("an audit entry was deleted")
	}

	// Tampering behind the triggers' back is detected. Each case runs in a
	// transaction that is rolled back.
	tamper := func(name string, stmts ...string) AuditVerification {
		t.Helper()
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		for _, q := range stmts {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %s: %v", name, q, err)
			}
		}
		v, err := verifyAudit(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		if v.OK {
			t.Errorf("%s: tampering not detected: %+v", name, v)
		}
		return v
	}
	idS := strconv.FormatInt(id, 10)
	seqS := strconv.FormatInt(seq, 10)
	v := tamper("edited entry",
		`ALTER TABLE audit_chain DISABLE TRIGGER trg_audit_chain_immutable`,
		`UPDATE audit_chain SET actor='someone-else' WHERE seq=`+seqS)
	if v.BrokenAt != seq {
		t.Errorf("edited entry: broken at %d, want %d", v.BrokenAt, seq)
	}
	tamper("removed entry",
		`ALTER TABLE audit_chain DISABLE TRIGGER trg_audit_chain_immutable`,
		`DELETE FROM audit_chain WHERE seq=`+seqS)
	tamper("removed tail",
		`ALTER TABLE audit_chain DISABLE TRIGGER trg_audit_chain_immutable`,
		`DELETE FROM audit_chain WHERE seq=(SELECT max(seq) FROM audit_chain)`)
	v = tamper("history row edited with the triggers off",
		`ALTER TABLE intercept_pending DISABLE TRIGGER trg_intercept_pending_audit`,
		`UPDATE intercept_pending SET status='denied' WHERE id=`+idS)
	if len(v.RowsMismatched) != 1 || v.RowsMismatched[0] != id || v.BrokenAt != 0 {
		t.Errorf("row edit: %+v", v)
	}
}
