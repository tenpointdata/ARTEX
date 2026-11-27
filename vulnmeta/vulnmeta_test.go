package vulnmeta

import (
	"errors"
	"testing"
)

// Reference scores from the FIRST CVSS v4.0 calculator.
func TestScoreCVSS4(t *testing.T) {
	cases := []struct {
		vector string
		score  float64
		rating string
	}{
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:H/SI:H/SA:H", 10.0, "critical"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N", 9.3, "critical"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:N/VI:N/VA:N/SC:N/SI:N/SA:N", 0.0, "none"},
		{"  CVSS:4.0/AV:L/AC:L/AT:N/PR:L/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N ", 8.5, "high"},
	}
	for _, c := range cases {
		got, err := ScoreCVSS4(c.vector)
		if err != nil {
			t.Fatalf("%s: %v", c.vector, err)
		}
		if got.Score != c.score || got.Rating != c.rating {
			t.Errorf("%s: got %v/%s, want %v/%s", c.vector, got.Score, got.Rating, c.score, c.rating)
		}
	}
	for _, bad := range []string{"", "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "CVSS:4.0/AV:N", "CVSS:4.0/AV:X/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"} {
		if _, err := ScoreCVSS4(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: want ErrInvalid, got %v", bad, err)
		}
	}
}

// A caller-supplied score is ignored: the stored score always matches the vector.
func TestNormalizeRecomputesScore(t *testing.T) {
	c, err := Normalize(Classification{CVSS: &CVSS{Vector: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N", Score: 1.0, Rating: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.CVSS.Score != 9.3 || c.CVSS.Rating != "critical" {
		t.Fatalf("score not recomputed: %+v", c.CVSS)
	}
	if c, _ := Normalize(Classification{CVSS: &CVSS{Vector: " "}}); c.CVSS != nil {
		t.Fatal("an empty vector must drop the CVSS")
	}
}

func TestNormalizeEPSS(t *testing.T) {
	c, err := Normalize(Classification{EPSS: &EPSS{Probability: 0.97, Percentile: 0.999, Date: "2026-11-20", CVE: "cve-2021-44228"}})
	if err != nil || c.EPSS.CVE != "CVE-2021-44228" {
		t.Fatalf("got %+v, %v", c.EPSS, err)
	}
	if c, err := Normalize(Classification{EPSS: &EPSS{}}); err != nil || c.EPSS != nil {
		t.Fatalf("empty epss must clear: %+v %v", c.EPSS, err)
	}
	for _, e := range []EPSS{{Probability: 1.2}, {Probability: -0.1}, {Probability: 0.5, Percentile: 2}, {Probability: 0.1, Date: "20/11/2026"}, {Probability: 0.1, CVE: "log4shell"}} {
		if _, err := Normalize(Classification{EPSS: &e}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: want ErrInvalid, got %v", e, err)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	got, err := NormalizeTags([]Tag{
		{"mitre-attack", "t1190"}, {"CWE", "89"}, {"owasp-wstg", "wstg-inpv-05"},
		{"owasp-asvs", "V5.3.4"}, {"cwe", "CWE-89"}, {"mitre-attack", "T1059.007"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Tag{{"cwe", "CWE-89"}, {"owasp-asvs", "V5.3.4"}, {"owasp-wstg", "WSTG-INPV-05"}, {"mitre-attack", "T1059.007"}, {"mitre-attack", "T1190"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	for _, bad := range []Tag{{"nist", "AC-2"}, {"cwe", "XSS"}, {"owasp-wstg", "INPV-05"}, {"mitre-attack", "T19"}, {"owasp-asvs", "5.3.4"}} {
		if _, err := NormalizeTags([]Tag{bad}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestParseTag(t *testing.T) {
	for in, want := range map[string]Tag{
		"CWE-79": {"cwe", "CWE-79"}, "wstg-athz-02": {"owasp-wstg", "WSTG-ATHZ-02"},
		"T1078": {"mitre-attack", "T1078"}, "owasp-asvs:v4.1.1": {"owasp-asvs", "V4.1.1"}, "cwe:22": {"cwe", "CWE-22"},
	} {
		got, err := ParseTag(in)
		if err != nil || got != want {
			t.Errorf("%q: got %v %v, want %v", in, got, err, want)
		}
	}
	if _, err := ParseTag("V5.3.4"); !errors.Is(err, ErrInvalid) {
		t.Error("an ASVS id without its framework must not be guessed")
	}
}

func TestPatchApply(t *testing.T) {
	base, _ := Normalize(Classification{Remediation: "fix it", Tags: []Tag{{"cwe", "79"}}, EPSS: &EPSS{Probability: 0.2}})
	vec := "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"
	got, err := Patch{CVSSVector: &vec, EPSS: &EPSS{}}.Apply(base)
	if err != nil {
		t.Fatal(err)
	}
	if got.CVSS == nil || got.CVSS.Score != 9.3 || got.EPSS != nil || got.Remediation != "fix it" || len(got.Tags) != 1 {
		t.Fatalf("patch result %+v", got)
	}
	empty, clear := "", []Tag{}
	got, _ = Patch{CVSSVector: &empty, Remediation: &empty, Tags: &clear}.Apply(got)
	if !got.IsZero() {
		t.Fatalf("clearing patch left %+v", got)
	}
	if !(Patch{}).IsEmpty() {
		t.Fatal("zero patch must be empty")
	}
}
