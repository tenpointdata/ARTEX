package scanimport

import (
	"path"
	"regexp"
	"strings"
)

// Lane says which testing lane a finding came from. Runtime scanners (nmap,
// nuclei, ZAP, Burp) test a running target; code scanners read source
// (SAST) or a dependency manifest (SCA). The lane is derived from typed fields
// only (tool name, rule id, file name), never from free text.
type Lane string

// Lanes.
const (
	LaneRuntime Lane = "runtime"
	LaneSAST    Lane = "sast"
	LaneSCA     Lane = "sca"
)

// Lanes lists the lanes in a stable order.
func Lanes() []Lane { return []Lane{LaneRuntime, LaneSAST, LaneSCA} }

// IsCode reports whether the lane reads code rather than a running target.
func (l Lane) IsCode() bool { return l == LaneSAST || l == LaneSCA }

// advisoryIDRe matches the vulnerability-database identifiers SCA tools use
// as rule ids: CVE, GHSA, OSV ecosystem ids (PYSEC, RUSTSEC, GO, GSD, ...),
// and Snyk ids.
var advisoryIDRe = regexp.MustCompile(`(?i)^(cve-\d{4}-\d{4,}|ghsa(-[23456789cfghjmpqrvwx]{4}){3}|(pysec|rustsec|go|gsd|osv|mal|bit|dsa|dla|usn|rhsa|alsa)-[0-9a-z-]+|snyk-[a-z0-9-]+)$`)

// scaTools are tool names (as normalized by toolName) that only report
// vulnerable dependencies.
var scaTools = map[string]bool{
	"osv-scanner": true, "osv-scanner-sarif": true, "grype": true, "dependency-check": true,
	"owasp-dependency-check": true, "snyk-open-source": true, "npm-audit": true, "pip-audit": true,
	"govulncheck": true, "cargo-audit": true, "bundler-audit": true, "retire.js": true, "dependabot": true,
}

// manifestFiles are dependency manifests and lockfiles: a result located in
// one is about a dependency, not about first-party code.
var manifestFiles = map[string]bool{
	"package.json": true, "package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true,
	"pnpm-lock.yaml": true, "bun.lockb": true, "requirements.txt": true, "pipfile": true, "pipfile.lock": true,
	"poetry.lock": true, "pyproject.toml": true, "uv.lock": true, "setup.py": true, "go.mod": true, "go.sum": true,
	"cargo.toml": true, "cargo.lock": true, "gemfile": true, "gemfile.lock": true, "composer.json": true,
	"composer.lock": true, "pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "gradle.lockfile": true,
	"packages.config": true, "packages.lock.json": true, "mix.lock": true, "pubspec.lock": true, "podfile.lock": true,
}

// codeLane classifies a code-scanner result. A result is SCA when the tool
// only does dependency scanning, the rule id is an advisory id, or the
// location is a manifest or lockfile; otherwise it is SAST.
func codeLane(tool, ruleID, location string) Lane {
	if scaTools[tool] || advisoryIDRe.MatchString(ruleID) || manifestFiles[strings.ToLower(path.Base(location))] {
		return LaneSCA
	}
	return LaneSAST
}

// LaneOf returns the lane of a format's findings when the format fixes it,
// and "" for SARIF, whose results carry their own lane.
func LaneOf(f Format) Lane {
	switch f {
	case NmapXML, NucleiJSONL, ZAPJSON, BurpXML:
		return LaneRuntime
	}
	return ""
}
