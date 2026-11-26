package server

import (
	"encoding/json"
	"testing"
)

// TestLLMProfileSamplingPinAPI drives the sampling pin through POST/GET
// /api/llm/profiles: a valid pin is stored and returned, a seed on a format
// whose API has no seed field and an out-of-range temperature are refused with
// 400 and leave nothing stored, and a temperature-only pin is valid on anthropic.
func TestLLMProfileSamplingPinAPI(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.PG()
	t.Cleanup(func() { pg.Exec(`DELETE FROM llm_profiles WHERE name LIKE 't-sampling-api%'`) })

	pinned := func(name string) *LLMProfileDTO {
		t.Helper()
		var list struct{ Profiles []LLMProfileDTO }
		if err := json.Unmarshal(request("GET", "/api/llm/profiles", "").Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		for i := range list.Profiles {
			if list.Profiles[i].Name == name {
				return &list.Profiles[i]
			}
		}
		return nil
	}
	save := func(body string, want int) {
		t.Helper()
		if resp := request("POST", "/api/llm/profiles", body); resp.Code != want {
			t.Fatalf("POST %s: want HTTP %d, got %d %s", body, want, resp.Code, resp.Body.String())
		}
	}

	save(`{"name":"t-sampling-api","format":"openai","model":"m","api_key":"k","sampling":{"seed":42,"temperature":0.2}}`, 200)
	p := pinned("t-sampling-api")
	if p == nil || p.Sampling.Seed == nil || *p.Sampling.Seed != 42 || p.Sampling.Temperature == nil || *p.Sampling.Temperature != 0.2 {
		t.Fatalf("stored pin not returned: %+v", p)
	}

	for _, body := range []string{
		`{"name":"t-sampling-api-anthropic","format":"anthropic","model":"m","api_key":"k","sampling":{"seed":1}}`,
		`{"name":"t-sampling-api-responses","format":"openai-responses","model":"m","api_key":"k","sampling":{"seed":1}}`,
		`{"name":"t-sampling-api-hot","format":"openai","model":"m","api_key":"k","sampling":{"temperature":2.5}}`,
		`{"name":"t-sampling-api-neg","format":"openai","model":"m","api_key":"k","sampling":{"temperature":-0.1}}`,
	} {
		save(body, 400)
	}
	for _, name := range []string{"t-sampling-api-anthropic", "t-sampling-api-responses", "t-sampling-api-hot", "t-sampling-api-neg"} {
		if pinned(name) != nil {
			t.Fatalf("refused profile %s was stored", name)
		}
	}

	// A temperature-only pin is valid on anthropic (its API has temperature, not seed).
	save(`{"name":"t-sampling-api-anthropic-t","format":"anthropic","model":"m","api_key":"k","sampling":{"temperature":0}}`, 200)
	if p := pinned("t-sampling-api-anthropic-t"); p == nil || p.Sampling.Temperature == nil || *p.Sampling.Temperature != 0 || p.Sampling.Seed != nil {
		t.Fatalf("temperature-only pin on anthropic: %+v", p)
	}
}
