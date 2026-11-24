package db

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScrubLessonText(t *testing.T) {
	terms := []string{"*.AcmeCorp.com", "Acme Corp", "10.9.8.0"}
	cases := []struct{ in, banned string }{
		{"XSS on https://shop.acmecorp.com/search?q=1 via q", "acmecorp"},
		{"admin portal at login.ACMECORP.com reachable", "acmecorp"},
		{"contact bob@victim.example.org for reset", "victim"},
		{"server 203.0.113.77:8443 leaks stack traces", "203.0.113.77"},
		{"cidr 198.51.100.0/24 exposed", "198.51.100"},
		{"v6 2001:db8::1 answered", "2001:db8"},
		{"Acme Corp staging box", "Acme Corp"},
		{"token=abcDEF1234567890abcDEF1234 accepted", "abcDEF1234567890"},
		{"Authorization: Bearer s3cr3tValue", "s3cr3tValue"},
		{"password: hunter2 worked", "hunter2"},
		{"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123 replayed", "eyJhbGci"},
		{"key AKIAIOSFODNN7EXAMPLE1234567890ab found", "AKIAIOSFODNN7"},
		{"internal.corp.example host", "internal.corp"},
	}
	for _, c := range cases {
		got := ScrubLessonText(c.in, terms)
		if strings.Contains(strings.ToLower(got), strings.ToLower(c.banned)) {
			t.Errorf("scrub(%q) = %q, still contains %q", c.in, got, c.banned)
		}
	}
	// Ordinary lesson text survives.
	for _, keep := range []string{
		"reflected xss in search parameter",
		"Confirmed sql-injection: error based, config.php exposes stack",
		"Recheck before reporting xss-stored-comment-field: a prior retest was fixed",
	} {
		if got := ScrubLessonText(keep, terms); got != keep {
			t.Errorf("scrub changed benign text %q -> %q", keep, got)
		}
	}
}

func TestRenderLessonsMarksCrossScope(t *testing.T) {
	block, _ := RenderLessons([]Lesson{{ID: 1, Kind: "positive", Cue: "xss", Support: 1, CrossScope: true,
		Action: "seen at https://other.example.net/a and 192.0.2.5"}}, 2000)
	if !strings.Contains(block, "other engagement") || !strings.Contains(block, "not instructions") {
		t.Fatalf("cross-scope lesson not marked or framing lost:\n%s", block)
	}
	if strings.Contains(block, "other.example.net") || strings.Contains(block, "192.0.2.5") {
		t.Fatalf("cross-scope render leaked target data:\n%s", block)
	}
}

// Two tasks with no company and different declared targets must not see each
// other's lessons; a same-target task does; the shared opt-in adds only
// scrubbed lessons from other engagements, without evidence ids.
func TestTaskLessonsStayInsideEngagement(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })

	stamp := time.Now().UnixNano()
	host1 := fmt.Sprintf("alpha-%d.victim-one.test", stamp)
	host2 := fmt.Sprintf("beta-%d.victim-two.test", stamp)
	mk := func(name, domain string) *Task {
		task, err := d.CreateTaskWithOptions(name, "goal", TaskCreateOptions{Name: fmt.Sprintf("%s-%d", name, stamp)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cleanupErr(t, name, d.DeleteTask(task.ID)) })
		if _, err := d.Exec(`INSERT INTO task_scope(task_id,kind,domain,source) VALUES ($1,'root_domain',$2,'manual')`,
			task.ID, domain); err != nil {
			t.Fatal(err)
		}
		return task
	}
	one := mk("scope-one", host1)
	oneAgain := mk("scope-one-again", host1)
	two := mk("scope-two", host2)

	var findingID int64
	if err := d.QueryRow(`
		INSERT INTO findings(task_id,vulnclass,name,summary,severity,status)
		VALUES ($1,'xss','Reflected XSS',$2,'high','confirmed') RETURNING id`, one.ID,
		fmt.Sprintf("reflected on https://%s/search?q=1 as admin@victim-one.test token=Zx9Qw8Er7Ty6Ui5Op4As3Df2", host1)).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupExec(t, d, `DELETE FROM findings WHERE id=$1`, findingID) })
	if n, err := d.ExtractLessonsForTask(t.Context(), one.ID); err != nil || n == 0 {
		t.Fatalf("extract n=%d err=%v", n, err)
	}
	t.Cleanup(func() {
		cleanupExec(t, d, `DELETE FROM rsi_lessons WHERE id IN (SELECT lesson_id FROM rsi_lesson_evidence WHERE source_kind='finding' AND source_id=$1)`, findingID)
	})

	// Written text carries no target data.
	rows, err := d.Query(`
		SELECT l.cue || ' ' || l.action, l.scrubbed, l.scope_key
		FROM rsi_lessons l JOIN rsi_lesson_evidence e ON e.lesson_id=l.id
		WHERE e.source_kind='finding' AND e.source_id=$1`, findingID)
	if err != nil {
		t.Fatal(err)
	}
	var stored int
	for rows.Next() {
		var text, key string
		var scrubbed bool
		if err := rows.Scan(&text, &scrubbed, &key); err != nil {
			t.Fatal(err)
		}
		stored++
		for _, banned := range []string{"victim-one", "alpha-", "admin@", "Zx9Qw8Er7"} {
			if strings.Contains(text, banned) {
				t.Errorf("lesson text leaked %q: %s", banned, text)
			}
		}
		if !scrubbed || !strings.HasPrefix(key, "scope:") {
			t.Errorf("scrubbed=%v scope_key=%q, want scrubbed and a scope: key", scrubbed, key)
		}
	}
	rows.Close()
	if stored == 0 {
		t.Fatal("no lesson stored")
	}

	own, err := d.TaskLessons(t.Context(), one.ID, 20)
	if err != nil || len(own) == 0 {
		t.Fatalf("source task sees %d lessons err=%v", len(own), err)
	}
	same, err := d.TaskLessons(t.Context(), oneAgain.ID, 20)
	if err != nil || len(same) == 0 {
		t.Fatalf("same-target task sees %d lessons err=%v", len(same), err)
	}
	other, err := d.TaskLessons(t.Context(), two.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range other {
		if l.ID == own[0].ID {
			t.Fatalf("different-target task saw lesson %d by default", l.ID)
		}
	}

	shared, err := d.TaskLessonsShared(t.Context(), two.ID, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	var sawShared bool
	for _, l := range shared {
		if l.ID == own[0].ID {
			sawShared = true
			if !l.CrossScope || len(l.Evidence) != 0 || l.CompanyID != 0 {
				t.Fatalf("cross-scope lesson not marked or leaks evidence: %+v", l)
			}
		}
	}
	if !sawShared {
		t.Fatal("shared mode did not offer the scrubbed lesson")
	}

	// An unscrubbed (pre-scoping) lesson is never offered across engagements.
	var legacyID int64
	if err := d.QueryRow(`
		INSERT INTO rsi_lessons(dedupe_key,kind,cue,action)
		VALUES ($1,'positive','xss','legacy text with raw host '||$2) RETURNING id`,
		fmt.Sprintf("legacy-%d", stamp), host1).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupExec(t, d, `DELETE FROM rsi_lessons WHERE id=$1`, legacyID) })
	for _, task := range []*Task{one, two} {
		for _, sharedMode := range []bool{false, true} {
			got, err := d.TaskLessonsShared(t.Context(), task.ID, 50, sharedMode)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range got {
				if l.ID == legacyID {
					t.Fatalf("legacy unscoped lesson selected (task %d shared=%v)", task.ID, sharedMode)
				}
			}
		}
	}
}

// A company lesson is not offered to another company's task even in shared
// mode unless it was scrubbed, and then only without its company id.
func TestCompanyLessonsNotSharedByDefault(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { d.Close() })

	stamp := time.Now().UnixNano()
	cs := d.Companies()
	coA, _, err := cs.UpsertCompany(fmt.Sprintf("ScrubCoA %d", stamp), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCompany(d, coA) })
	coB, _, err := cs.UpsertCompany(fmt.Sprintf("ScrubCoB %d", stamp), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCompany(d, coB) })
	taskA, err := d.CreateTaskWithOptions("co a", "goal", TaskCreateOptions{Name: fmt.Sprintf("co-a-%d", stamp), CompanyIDs: []int64{coA}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "task a", d.DeleteTask(taskA.ID)) })
	taskB, err := d.CreateTaskWithOptions("co b", "goal", TaskCreateOptions{Name: fmt.Sprintf("co-b-%d", stamp), CompanyIDs: []int64{coB}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupErr(t, "task b", d.DeleteTask(taskB.ID)) })

	var findingID int64
	if err := d.QueryRow(`
		INSERT INTO findings(task_id,vulnclass,name,summary,severity,status)
		VALUES ($1,'idor','IDOR',$2,'high','confirmed') RETURNING id`, taskA.ID,
		fmt.Sprintf("ScrubCoA %d customer data readable at 203.0.113.9", stamp)).Scan(&findingID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupExec(t, d, `DELETE FROM findings WHERE id=$1`, findingID) })
	if _, err := d.ExtractLessonsForTask(t.Context(), taskA.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupExec(t, d, `DELETE FROM rsi_lessons WHERE id IN (SELECT lesson_id FROM rsi_lesson_evidence WHERE source_kind='finding' AND source_id=$1)`, findingID)
	})

	ownA, err := d.TaskLessons(t.Context(), taskA.ID, 20)
	if err != nil || len(ownA) == 0 {
		t.Fatalf("company A sees %d lessons err=%v", len(ownA), err)
	}
	for _, l := range ownA {
		if strings.Contains(l.Action, "ScrubCoA") || strings.Contains(l.Action, "203.0.113.9") {
			t.Fatalf("company lesson kept target data: %q", l.Action)
		}
	}
	gotB, err := d.TaskLessons(t.Context(), taskB.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range gotB {
		if l.ID == ownA[0].ID {
			t.Fatal("company B saw company A's lesson by default")
		}
	}
	sharedB, err := d.TaskLessonsShared(t.Context(), taskB.ID, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, l := range sharedB {
		if l.ID == ownA[0].ID {
			saw = true
			if !l.CrossScope || l.CompanyID != 0 || len(l.Evidence) != 0 {
				t.Fatalf("shared company lesson leaks scope: %+v", l)
			}
		}
	}
	if !saw {
		t.Fatal("shared mode did not offer company A's scrubbed lesson to company B")
	}
}
