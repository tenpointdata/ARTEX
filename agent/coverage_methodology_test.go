package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/methodology"
)

func TestResolveMethodologyItem(t *testing.T) {
	all, err := methodology.All()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := resolveMethodologyItem(all, " wstg-inpv-05 "); err != nil || got != "WSTG-INPV-05" {
		t.Fatalf("canonical id: %q %v", got, err)
	}
	if got, err := resolveMethodologyItem(all, "api1:2023"); err != nil || got != "API1:2023" {
		t.Fatalf("api id: %q %v", got, err)
	}
	if got, err := resolveMethodologyItem(nil, ""); err != nil || got != "" {
		t.Fatalf("empty citation is allowed: %q %v", got, err)
	}
	if _, err := resolveMethodologyItem(all, "sqli"); err == nil {
		t.Fatal("an alias is not an item id: refused so citations stay unambiguous")
	}
	if _, err := resolveMethodologyItem(nil, "WSTG-INPV-05"); err == nil || !strings.Contains(err.Error(), "no methodology applies") {
		t.Fatalf("no methodology: %v", err)
	}
}

func TestRenderMethodologyBlock(t *testing.T) {
	if renderMethodologyBlock(nil) != "" {
		t.Fatal("no methodology, no block")
	}
	m, _ := methodology.Get("owasp-api-top10")
	s := methodology.Evaluate(m, []methodology.Observation{{RiskArea: "bola", Outcome: "no_issue_found", Closes: true}},
		[]methodology.Citation{{ItemID: "API2:2023", IntentID: 41}})
	out := renderMethodologyBlock([]methodology.Score{s})
	for _, want := range []string{
		"[Methodology coverage]", "methodology_item", "covered 1/10, in progress 1, open 8",
		"- API2:2023 Broken authentication [in_progress] intents #41", "- and 3 more not covered",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("block lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "API1:2023 Broken object level") {
		t.Errorf("a covered item is not listed as pending:\n%s", out)
	}
}

func TestCompactIntentsCarriesMethodologyItem(t *testing.T) {
	n := &db.Node{ID: 7, Kind: db.KindIntent, Payload: json.RawMessage(`{"summary":"s","methodology_item":"WSTG-INPV-05"}`)}
	got := compactIntents([]*db.Node{n}, nil, nil)
	if got[0]["methodology_item"] != "WSTG-INPV-05" {
		t.Fatalf("%v", got[0])
	}
}
