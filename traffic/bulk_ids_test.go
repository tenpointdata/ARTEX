package traffic

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// seedLegacyExchange inserts an index row whose bodies live in a legacy
// per-exchange directory host/GET/id, and creates that directory.
func seedLegacyExchange(t *testing.T, tr *Traffic, dir, id, host string) string {
	t.Helper()
	rel := filepath.Join(host, "GET", id)
	if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel, "request.http"), []byte("GET / HTTP/1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.DB().Exec(`INSERT INTO exchanges(id,ts,host,method,url_template,url,status,content_type,req_len,resp_len,path)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, 1, host, "GET", "/", "http://"+host+"/", 200, "text/html", 0, 0, filepath.ToSlash(rel)); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, rel)
}

// TestDeleteIDsDeletesExactlyTheSelection: only the named exchanges go — a
// sibling on the same host stays, along with its directory — and blanks,
// duplicates and unknown ids are harmless.
func TestDeleteIDsDeletesExactlyTheSelection(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	a := seedLegacyExchange(t, tr, dir, "1-0001", "api.example.com")
	b := seedLegacyExchange(t, tr, dir, "1-0002", "api.example.com")
	c := seedLegacyExchange(t, tr, dir, "1-0003", "shop.example.com")

	n, err := tr.DeleteIDs([]string{"1-0001", " 1-0001 ", "", "1-0003", "no-such-id"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted=%d, want 2", n)
	}
	var left []string
	rows, err := tr.DB().Query(`SELECT id FROM exchanges ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		left = append(left, id)
	}
	rows.Close()
	if len(left) != 1 || left[0] != "1-0002" {
		t.Fatalf("remaining=%v, want [1-0002]", left)
	}
	tr.reaping.Wait()
	for _, gone := range []string{a, c} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", gone, err)
		}
	}
	if _, err := os.Stat(b); err != nil {
		t.Fatalf("sibling directory %s removed: %v", b, err)
	}
}

func TestDeleteIDsRejectsTooMany(t *testing.T) {
	tr, err := Open(t.TempDir(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ids := make([]string, MaxBulkIDs+1)
	for i := range ids {
		ids[i] = "id-" + strconv.Itoa(i)
	}
	if _, err := tr.DeleteIDs(ids); err != ErrTooManyIDs {
		t.Fatalf("err=%v, want ErrTooManyIDs", err)
	}
}

// TestLegacyExchangeDirRejectsUnsafePaths: a corrupt path never stages a whole
// host tree, the store's own directories, or anything outside the store.
func TestLegacyExchangeDirRejectsUnsafePaths(t *testing.T) {
	tr := &Traffic{dir: "/store"}
	for _, rel := range []string{"api.example.com", "../etc/GET/x", "/abs/GET/x", "_blobs/ab/cd", "a/../../x", "."} {
		if dir, ok := tr.legacyExchangeDir(rel); ok {
			t.Fatalf("legacyExchangeDir(%q) = %q, want rejected", rel, dir)
		}
	}
	if _, ok := tr.legacyExchangeDir("api.example.com/GET/1-0001"); !ok {
		t.Fatal("a host/method/id path was rejected")
	}
}

// TestExportHARByIDs: the IDs filter exports exactly the selection.
func TestExportHARByIDs(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	for _, id := range []string{"1-0001", "1-0002", "1-0003"} {
		seedLegacyExchange(t, tr, dir, id, "api.example.com")
	}
	doc, meta, err := tr.ExportHAR(PageQuery{IDs: []string{"1-0001", "1-0003"}, RespMin: -1, RespMax: -1}, HARExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Exported != 2 || meta.TotalMatched != 2 || len(doc.Log.Entries) != 2 {
		t.Fatalf("meta=%+v entries=%d, want 2/2/2", meta, len(doc.Log.Entries))
	}
}
