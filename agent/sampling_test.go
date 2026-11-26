package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/llm"
)

func pinOf[T any](v T) *T { return &v }

// sentBody builds a real provider for format with the pin, sends one request and
// returns the top-level JSON the endpoint received. The endpoint answers 400 so
// the test needs no format-specific response; only the request body matters.
func sentBody(t *testing.T, format string, pin db.Sampling, req llm.CompletionRequest) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("endpoint got a body that is not JSON: %v: %s", err, b)
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"stop here"}}`)
	}))
	defer srv.Close()
	cfg := ConfigFrom(format, "test-model", srv.URL, "test-key", "")
	cfg.Sampling = pin
	p, err := cfg.NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	req.Messages = []llm.Message{llm.UserText("ping")}
	_, _, _, _ = p.Complete(context.Background(), req)
	if got == nil {
		t.Fatal("endpoint saw no request")
	}
	return got
}

func TestProfileSeedAndTemperatureReachTheProvider(t *testing.T) {
	got := sentBody(t, "openai", db.Sampling{Seed: pinOf(int64(42)), Temperature: pinOf(0.25)}, llm.CompletionRequest{})
	if string(got["seed"]) != "42" || string(got["temperature"]) != "0.25" {
		t.Fatalf("seed=%s temperature=%s, want 42 and 0.25", got["seed"], got["temperature"])
	}
	if string(got["model"]) != `"test-model"` || got["messages"] == nil {
		t.Fatalf("the rest of the body was damaged: %v", got)
	}
}

func TestOllamaProfileSendsSeed(t *testing.T) {
	got := sentBody(t, "ollama", db.Sampling{Seed: pinOf(int64(-3))}, llm.CompletionRequest{})
	if string(got["seed"]) != "-3" {
		t.Fatalf("seed=%s, want -3", got["seed"])
	}
	if _, set := got["temperature"]; set {
		t.Fatalf("temperature sent without a pin: %s", got["temperature"])
	}
}

func TestCallerTemperatureWinsOverThePin(t *testing.T) {
	got := sentBody(t, "openai", db.Sampling{Temperature: pinOf(1.5)}, llm.CompletionRequest{Temperature: pinOf(0.0)})
	if string(got["temperature"]) != "0" {
		t.Fatalf("temperature=%s, want the caller's 0", got["temperature"])
	}
}

func TestAnthropicProfileGetsTemperatureButNoSeed(t *testing.T) {
	got := sentBody(t, "anthropic", db.Sampling{Seed: pinOf(int64(1)), Temperature: pinOf(0.5)}, llm.CompletionRequest{MaxTokens: 16})
	if _, set := got["seed"]; set {
		t.Fatal("seed sent to a format whose API rejects it")
	}
	if string(got["temperature"]) != "0.5" {
		t.Fatalf("temperature=%s, want 0.5", got["temperature"])
	}
}

func TestUnpinnedProfileSendsNeither(t *testing.T) {
	got := sentBody(t, "openai", db.Sampling{}, llm.CompletionRequest{})
	for _, k := range []string{"seed", "temperature"} {
		if _, set := got[k]; set {
			t.Fatalf("%s sent without a pin", k)
		}
	}
}

func TestWithSamplingFields(t *testing.T) {
	f := map[string]string{"seed": "7", "temperature": "0.1"}
	cases := map[string]string{
		`{"model":"m"}`:            `{"seed":7,"temperature":0.1,"model":"m"}`,
		`{}`:                       `{"seed":7,"temperature":0.1}`,
		` { }`:                     ` {"seed":7,"temperature":0.1 }`,
		`{"seed":1,"model":"m"}`:   `{"temperature":0.1,"seed":1,"model":"m"}`,
		`[1,2]`:                    `[1,2]`,
		`not json`:                 `not json`,
		`{"a":{"seed":1}}`:         `{"seed":7,"temperature":0.1,"a":{"seed":1}}`,
		`{"temperature":2,"x":[]}`: `{"seed":7,"temperature":2,"x":[]}`,
	}
	for in, want := range cases {
		out := string(withSamplingFields([]byte(in), f))
		if out != want {
			t.Errorf("%s -> %s, want %s", in, out, want)
		}
		if json.Valid([]byte(in)) && !json.Valid([]byte(out)) {
			t.Errorf("%s -> %s is not valid JSON", in, out)
		}
	}
}

func TestValidateSampling(t *testing.T) {
	ok := []struct {
		format string
		s      db.Sampling
	}{
		{"openai", db.Sampling{}},
		{"openai", db.Sampling{Seed: pinOf(int64(1)), Temperature: pinOf(0.0)}},
		{"ollama", db.Sampling{Seed: pinOf(int64(1))}},
		{"anthropic", db.Sampling{Temperature: pinOf(2.0)}},
	}
	for _, c := range ok {
		if err := ValidateSampling(c.format, c.s); err != nil {
			t.Errorf("%s %+v: %v", c.format, c.s, err)
		}
	}
	bad := []struct {
		format string
		s      db.Sampling
	}{
		{"anthropic", db.Sampling{Seed: pinOf(int64(1))}},
		{"openai-responses", db.Sampling{Seed: pinOf(int64(1))}},
		{"openai", db.Sampling{Temperature: pinOf(-0.1)}},
		{"openai", db.Sampling{Temperature: pinOf(2.5)}},
	}
	for _, c := range bad {
		if err := ValidateSampling(c.format, c.s); err == nil {
			t.Errorf("%s %+v: accepted", c.format, c.s)
		}
	}
}
