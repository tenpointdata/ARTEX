package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoSendsJSONAndReturnsResponse(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "1")
	var gotBody, gotCT, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotCT, gotAuth, gotMethod = string(b), r.Header.Get("Content-Type"), r.Header.Get("Authorization"), r.Method
		w.Header().Set("X-Total", "3")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	resp, err := Do(context.Background(), Request{Method: "POST", URL: srv.URL + "/x", Headers: map[string]string{"Authorization": "Bearer t"}, Body: map[string]int{"a": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 201 || string(resp.Body) != `{"ok":true}` || resp.Header.Get("X-Total") != "3" {
		t.Fatalf("resp = %+v", resp)
	}
	if gotBody != `{"a":1}` || !strings.HasPrefix(gotCT, "application/json") || gotAuth != "Bearer t" || gotMethod != "POST" {
		t.Fatalf("server saw body=%q ct=%q auth=%q method=%q", gotBody, gotCT, gotAuth, gotMethod)
	}
}

func TestDoClassifiesErrorsAndKeepsResponse(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "1")
	for status, permanent := range map[int]bool{400: true, 401: true, 404: true, 408: false, 429: false, 500: false, 503: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("reason text"))
		}))
		resp, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL})
		srv.Close()
		if err == nil || IsPermanent(err) != permanent || !strings.Contains(err.Error(), "reason text") {
			t.Errorf("HTTP %d: err=%v permanent=%v", status, err, IsPermanent(err))
		}
		if resp == nil || resp.Status != status {
			t.Errorf("HTTP %d: response not returned alongside the error: %+v", status, resp)
		}
	}
}

func TestDoResponseLimit(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
	}))
	defer srv.Close()
	small, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL, Limit: 100})
	if err != nil || len(small.Body) != 100 {
		t.Fatalf("limit 100: len=%d err=%v", len(small.Body), err)
	}
	full, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil || len(full.Body) != 5000 {
		t.Fatalf("default limit: len=%d err=%v", len(full.Body), err)
	}
}

func TestDoNeverLeaksURLCredentials(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "")
	for _, target := range []string{"http://user:hunter2@127.0.0.1:1/p?token=hunter2", "http://127.0.0.1:1/bot-hunter2/send", "http://bad host/hunter2"} {
		_, err := Do(context.Background(), Request{Method: "GET", URL: target})
		if err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Errorf("Do(%q) err = %v", target, err)
		}
	}
	// Unmarshalable body is a permanent local error.
	if _, err := Do(context.Background(), Request{Method: "POST", URL: "http://127.0.0.1:1", Body: make(chan int)}); err == nil || !IsPermanent(err) {
		t.Fatalf("bad body err = %v", err)
	}
}

func TestMaskConfigKeysAndPrepareConfigUpdateKeys(t *testing.T) {
	masked := MaskConfigKeys([]string{"token"}, map[string]any{"token": "abcdefghijkl", "repo": "a/b", "n": 1})
	if masked["token"] != "__masked__:…ghijkl" || masked["repo"] != "a/b" {
		t.Fatalf("masked = %v", masked)
	}
	stored := map[string]any{"token": "real", "repo": "a/b"}
	merged, err := PrepareConfigUpdateKeys([]string{"token"}, []string{"repo"}, stored, map[string]any{"token": masked["token"], "repo": "a/b"})
	if err != nil || merged["token"] != "real" {
		t.Fatalf("unchanged destination: %v %v", merged, err)
	}
	_, err = PrepareConfigUpdateKeys([]string{"token"}, []string{"repo"}, stored, map[string]any{"token": masked["token"], "repo": "evil/x"})
	if _, ok := err.(*ErrDestinationChangedWithoutCredentials); !ok {
		t.Fatalf("err = %v", err)
	}
	merged, err = PrepareConfigUpdateKeys([]string{"token"}, []string{"repo"}, stored, map[string]any{"token": "new", "repo": "evil/x"})
	if err != nil || merged["token"] != "new" || merged["repo"] != "evil/x" {
		t.Fatalf("explicit credential: %v %v", merged, err)
	}
}
