package scanimport

import (
	"os"
	"testing"
)

func TestCodeLane(t *testing.T) {
	for _, tc := range []struct {
		tool, rule, loc string
		want            Lane
	}{
		{"semgrep-oss", "python.django.security.injection.sql", "app/views.py", LaneSAST},
		{"trivy", "CVE-2022-24999", "web/package-lock.json", LaneSCA},
		{"trivy", "aws-s3-enable-versioning", "infra/main.tf", LaneSAST},
		{"codeql", "GHSA-xvch-5gv4-984h", "src/a.js", LaneSCA},
		{"osv-scanner", "anything", "src/a.go", LaneSCA},
		{"semgrep", "ssc-1234", "go.sum", LaneSCA},
		{"semgrep", "go.lang.security.audit", "pkg/go.go", LaneSAST},
		{"snyk", "SNYK-JS-LODASH-567746", "x.js", LaneSCA},
	} {
		if got := codeLane(tc.tool, tc.rule, tc.loc); got != tc.want {
			t.Errorf("codeLane(%q,%q,%q) = %q, want %q", tc.tool, tc.rule, tc.loc, got, tc.want)
		}
	}
}

func TestFindingLanes(t *testing.T) {
	for _, tc := range []struct {
		format Format
		file   string
		want   map[Lane]int
	}{
		{SARIF, "semgrep.sarif", map[Lane]int{LaneSAST: 3}},
		{SARIF, "trivy-sca.sarif", map[Lane]int{LaneSCA: 2, LaneSAST: 1}},
		{NucleiJSONL, "nuclei.jsonl", nil},
	} {
		f, err := os.Open("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Parse(tc.format, f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := map[Lane]int{}
		for _, fd := range res.Findings {
			got[fd.Lane]++
		}
		if tc.want == nil { // a runtime format: every finding is runtime
			if len(res.Findings) == 0 || got[LaneRuntime] != len(res.Findings) {
				t.Errorf("%s: lanes %v", tc.file, got)
			}
			continue
		}
		for l, n := range tc.want {
			if got[l] != n {
				t.Errorf("%s: lane %s = %d, want %d (all %v)", tc.file, l, got[l], n, got)
			}
		}
	}
}
