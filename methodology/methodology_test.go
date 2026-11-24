package methodology

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuiltinLoadsAndValidates(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"owasp-wstg": 97, "owasp-api-top10": 10}
	if len(all) != len(want) {
		t.Fatalf("got %d methodologies, want %d", len(all), len(want))
	}
	for _, m := range all {
		if n, ok := want[m.ID]; !ok || len(m.Items) != n {
			t.Errorf("%s: %d items, want %d", m.ID, len(m.Items), n)
		}
		if m.Source == "" || m.Version == "" || len(m.AppliesWhen) == 0 {
			t.Errorf("%s: source, version and applies_when are required for the built-ins", m.ID)
		}
	}
	if _, ok := Get("owasp-wstg"); !ok {
		t.Fatal("Get(owasp-wstg) missing")
	}
	m, _ := Get("owasp-wstg")
	if it, ok := m.Item("wstg-inpv-05"); !ok || !strings.Contains(it.Title, "SQL") {
		t.Fatalf("Item lookup is case-insensitive: %+v %v", it, ok)
	}
}

func TestValidateRejectsAmbiguousAlias(t *testing.T) {
	m := Methodology{ID: "x", Name: "X", Items: []Item{
		{ID: "A", Title: "a", RiskAreas: []string{"xss"}},
		{ID: "B", Title: "b", RiskAreas: []string{"XSS"}},
	}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "names both") {
		t.Fatalf("want ambiguity error, got %v", err)
	}
	m.Items[1].RiskAreas = []string{"a"} // alias equal to another item's id
	if err := m.Validate(); err == nil {
		t.Fatal("an alias equal to another item id must be refused")
	}
}

func TestLoadRejectsUnknownFieldsAndDuplicates(t *testing.T) {
	one := `{"id":"m","name":"M","items":[{"id":"I","title":"t"}]}`
	if _, err := Load(fstest.MapFS{"a.json": {Data: []byte(`{"id":"m","name":"M","bogus":1,"items":[{"id":"I","title":"t"}]}`)}}); err == nil {
		t.Fatal("unknown field must be refused")
	}
	if _, err := Load(fstest.MapFS{"a.json": {Data: []byte(one)}, "b.json": {Data: []byte(one)}}); err == nil {
		t.Fatal("duplicate id must be refused")
	}
}

func TestSelect(t *testing.T) {
	all, _ := All()
	ids := func(ms []Methodology) string {
		var s []string
		for _, m := range ms {
			s = append(s, m.ID)
		}
		return strings.Join(s, ",")
	}
	cases := map[string]string{
		"Find vulnerabilities in https://shop.example.test":             "owasp-wstg",
		"Test the REST API at api.example.test for BOLA":                "owasp-api-top10",
		"Assess the GraphQL endpoint of https://app.example.test/graph": "owasp-api-top10,owasp-wstg",
		"Port-scan 10.0.0.0/24 and report open services":                "",
		"Capture the flag on the webapp":                                "owasp-wstg",
		"rapid enumeration":                                             "", // "api" only as a word
	}
	for text, want := range cases {
		if got := ids(Select(all, text)); got != want {
			t.Errorf("Select(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	m := Methodology{ID: "m", Name: "M", Items: []Item{
		{ID: "T-1", Title: "sqli", RiskAreas: []string{"sqli"}},
		{ID: "T-2", Title: "xss", RiskAreas: []string{"xss"}},
		{ID: "T-3", Title: "csrf", RiskAreas: []string{"csrf"}},
		{ID: "T-4", Title: "idor"},
	}}
	s := Evaluate(m, []Observation{
		{RiskArea: "SQLi", Outcome: "no_issue_found", Closes: true},
		{RiskArea: "xss", Outcome: "needs_follow_up"},
		{RiskArea: "t-4", Outcome: "reported", Closes: true}, // by item id
		{RiskArea: "rce", Outcome: "reported", Closes: true}, // matches nothing
	}, []Citation{{ItemID: "T-3", IntentID: 9}, {ItemID: "t-3", IntentID: 4}, {ItemID: "csrf", IntentID: 5}, {ItemID: "nope", IntentID: 6}})
	if s.Total != 4 || s.Covered != 2 || s.InProgress != 2 || s.Open != 0 || s.Pct != 0.5 {
		t.Fatalf("counts: %+v", s)
	}
	byID := map[string]ItemStatus{}
	for _, it := range s.Items {
		byID[it.ID] = it
	}
	if byID["T-1"].Status != StatusCovered || byID["T-2"].Status != StatusInProgress || byID["T-4"].Status != StatusCovered {
		t.Fatalf("statuses: %+v", s.Items)
	}
	if got := byID["T-3"]; got.Status != StatusInProgress || len(got.Intents) != 2 || got.Intents[0] != 4 || got.Intents[1] != 9 {
		t.Fatalf("citations count by item id only, sorted: %+v", got)
	}
	if p := s.Pending(5); len(p) != 2 || p[0].ID != "T-2" || p[1].ID != "T-3" {
		t.Fatalf("pending: %+v", p)
	}
	empty := Evaluate(m, nil, nil)
	if empty.Open != 4 || empty.Covered != 0 {
		t.Fatalf("absence covers nothing: %+v", empty)
	}
	if p := empty.Pending(2); len(p) != 2 || p[0].ID != "T-1" {
		t.Fatalf("pending caps at n: %+v", p)
	}
}

func TestResolveIDsAndPick(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveIDs(all, []string{" OWASP API Top10 ", "", "owasp-wstg", "owasp_api_top10"})
	if err != nil || len(got) != 2 || got[0] != "owasp-api-top10" || got[1] != "owasp-wstg" {
		t.Fatalf("ResolveIDs = %v, %v", got, err)
	}
	if _, err := ResolveIDs(all, []string{"owasp-nope"}); err == nil {
		t.Fatal("unknown id accepted")
	}
	if got, err := ResolveIDs(all, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty ResolveIDs = %#v, %v", got, err)
	}
	picked := Pick(all, []string{"owasp-wstg", "gone", "owasp-api-top10"})
	if len(picked) != 2 || picked[0].ID != "owasp-wstg" || picked[1].ID != "owasp-api-top10" {
		t.Fatalf("Pick = %+v", picked)
	}
}

func TestNoneChoice(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveIDs(all, []string{" NONE ", "none"}); err != nil || len(got) != 1 || got[0] != None {
		t.Fatalf("none alone: %v %v", got, err)
	}
	if _, err := ResolveIDs(all, []string{"owasp-wstg", "none"}); err == nil {
		t.Fatal("none beside a methodology must be refused")
	}
	web := "test https://shop.example.test"
	if got := Choose(all, []string{None}, web); len(got) != 0 {
		t.Fatalf("none chooses nothing: %v", got)
	}
	if got := Choose(all, nil, web); len(got) != 1 || got[0].ID != "owasp-wstg" {
		t.Fatalf("automatic: %v", got)
	}
	if got := Choose(all, []string{"owasp-api-top10"}, web); len(got) != 1 || got[0].ID != "owasp-api-top10" {
		t.Fatalf("chosen: %v", got)
	}
	if err := (Methodology{ID: "None", Name: "x", Items: []Item{{ID: "a", Title: "a"}}}).Validate(); err == nil {
		t.Fatal("the reserved id must not name a methodology")
	}
}
