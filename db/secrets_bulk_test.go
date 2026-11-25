package db

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

// rawLLMBodies reads the four body columns of one llm_records row as stored.
func rawLLMBodies(t *testing.T, d *DB, id int64) []string {
	t.Helper()
	out := make([]string, 4)
	if err := d.QueryRow(`SELECT COALESCE(request_body,''),COALESCE(response_body,''),COALESCE(raw_request,''),COALESCE(raw_response,'')
		FROM llm_records WHERE id=$1`, id).Scan(&out[0], &out[1], &out[2], &out[3]); err != nil {
		t.Fatal(err)
	}
	return out
}

func rawLogText(t *testing.T, d *DB, id int64) string {
	t.Helper()
	var v string
	if err := d.QueryRow(`SELECT text FROM server_logs WHERE id=$1`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func findLog(t *testing.T, logs []*DBLog, id int64) *DBLog {
	t.Helper()
	for _, l := range logs {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("log id=%d not listed", id)
	return nil
}

// LLM record bodies and log lines: ciphertext at rest, plaintext on read,
// legacy plaintext rows readable and sealed by the background backfill,
// metadata filters unaffected, a foreign key refused at startup.
func TestBulkSecretsAtRest(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	const task = "bulk-secrets-test"
	cleanup := func() {
		_, _ = d.Exec(`DELETE FROM llm_records WHERE task_id=$1`, task)
		_, _ = d.Exec(`DELETE FROM server_logs WHERE tag=$1`, task)
	}
	cleanup()
	t.Cleanup(cleanup)

	rec := &LLMRecord{Model: "m-bulk", SessionID: "s-bulk", TaskID: task, Status: "ok", InputTokens: 7,
		RequestBody: `{"messages":"Authorization: Bearer tok-req"}`, ResponseBody: "resp tok-resp",
		RawRequest: "raw tok-rawreq", RawResponse: "data: tok-rawresp"}
	if err := d.InsertLLMRecord(rec); err != nil {
		t.Fatal(err)
	}
	list, total, err := d.ListLLMRecords("m-bulk", "s-bulk", task, 0, 10)
	if err != nil || total != 1 || len(list) != 1 || list[0].InputTokens != 7 {
		t.Fatalf("metadata filter: total=%d %v", total, err)
	}
	id := list[0].ID
	for _, v := range rawLLMBodies(t, d, id) {
		if !isEncrypted(v) || strings.Contains(v, "tok-") {
			t.Fatalf("llm_records body stored in clear: %q", v)
		}
	}
	got, err := d.GetLLMRecord(id)
	if err != nil || got.RequestBody != rec.RequestBody || got.ResponseBody != rec.ResponseBody ||
		got.RawRequest != rec.RawRequest || got.RawResponse != rec.RawResponse {
		t.Fatalf("read back: %+v %v", got, err)
	}

	logID, err := d.InsertLog("info", task, "["+task+"] proxy http://u:log-secret@h")
	if err != nil {
		t.Fatal(err)
	}
	if v := rawLogText(t, d, logID); !isEncrypted(v) || strings.Contains(v, "log-secret") {
		t.Fatalf("server_logs.text stored in clear: %q", v)
	}
	logs, err := d.ListLogsBefore(logID+1, 5)
	if err != nil || findLog(t, logs, logID).Text != "["+task+"] proxy http://u:log-secret@h" {
		t.Fatalf("log read back: %v", err)
	}

	// Legacy plaintext rows, written before this change: readable as-is.
	var legacyID, legacyLog int64
	if err := d.QueryRow(`INSERT INTO llm_records(model,session_id,task_id,status,request_body,response_body,raw_request,raw_response)
		VALUES ('m-bulk','s-bulk',$1,'ok','legacy req','legacy resp','legacy rawreq',NULL) RETURNING id`, task).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`INSERT INTO server_logs(level,tag,text) VALUES ('info',$1,'legacy line') RETURNING id`, task).Scan(&legacyLog); err != nil {
		t.Fatal(err)
	}
	if got, err := d.GetLLMRecord(legacyID); err != nil || got.RequestBody != "legacy req" || got.RawResponse != "" {
		t.Fatalf("legacy read: %+v %v", got, err)
	}
	if logs, err := d.RecentLogs(50); err != nil || findLog(t, logs, legacyLog).Text != "legacy line" {
		t.Fatalf("legacy log read: %v", err)
	}

	// The backfill seals them in place (batch 1 exercises the cursor), once.
	if _, err := d.BackfillBulkSecrets(context.Background(), 1, 0); err != nil {
		t.Fatal(err)
	}
	raw := rawLLMBodies(t, d, legacyID)
	for _, v := range raw[:3] {
		if !isEncrypted(v) {
			t.Fatalf("backfill left %q in clear", v)
		}
	}
	if raw[3] != "" {
		t.Fatalf("NULL body must stay empty, got %q", raw[3])
	}
	if v := rawLogText(t, d, legacyLog); !isEncrypted(v) {
		t.Fatalf("backfill left log %q in clear", v)
	}
	if got, err := d.GetLLMRecord(legacyID); err != nil || got.ResponseBody != "legacy resp" {
		t.Fatalf("after backfill: %+v %v", got, err)
	}
	if n, err := d.BackfillBulkSecrets(context.Background(), 0, 0); err != nil || n != 0 {
		t.Fatalf("second backfill sealed %d (%v), want 0", n, err)
	}
	if raw2 := rawLLMBodies(t, d, legacyID); raw2[0] != raw[0] {
		t.Fatal("second backfill rewrote a sealed value")
	}

	// A body sealed under another key fails the startup key check.
	other, _ := newSecretBox([]byte("ffffffffffffffffffffffffffffffff"))
	foreign, _ := other.encrypt(colServerLogText.aad(), "foreign")
	if _, err := d.Exec(`INSERT INTO server_logs(level,tag,text) VALUES ('info',$1,$2)`, task, foreign); err != nil {
		t.Fatal(err)
	}
	t.Setenv(secretKeyEnv, base64.StdEncoding.EncodeToString(testSecretKey))
	if err := d.InitSecrets(t.TempDir()); err == nil || !strings.Contains(err.Error(), "server_logs.text") {
		t.Fatalf("want a key-check failure on server_logs.text, got %v", err)
	}
	if _, err := d.Exec(`DELETE FROM server_logs WHERE text=$1`, foreign); err != nil {
		t.Fatal(err)
	}
	if err := d.InitSecrets(t.TempDir()); err != nil {
		t.Fatalf("right key after the foreign row is gone: %v", err)
	}
}

// With only sealed bulk values (no other secret), a missing key is refused
// rather than a new key generated over them.
func TestBulkSealedExistsBlocksNewKey(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	logID, err := d.InsertLog("info", "bulk-exists-test", "sealed line")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM server_logs WHERE id=$1`, logID) })
	conn, err := d.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if ok, err := bulkSealedExists(context.Background(), conn); err != nil || !ok {
		t.Fatalf("bulkSealedExists = %v, %v", ok, err)
	}
}
