//go:build unix

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without the drop, the keys and the config still become 0600. Runs without root.
func TestRestrictSecretModes(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	files := []string{filepath.Join(dir, "jwt.key"), filepath.Join(dir, "secret.key"), cfg}
	for _, p := range files {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := PrepareExecPrivilege(&ExecPrivilege{}, dir, dir, cfg); err != nil {
		t.Fatal(err)
	}
	for _, p := range files {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v, want 0600", p, fi.Mode(), err)
		}
	}
}

// Only run directories get a home of their own.
func TestIsRunDir(t *testing.T) {
	data := filepath.FromSlash("/data")
	for rel, want := range map[string]bool{
		"tasks/1/main":     true,
		"tasks/12/i3":      true,
		"sessions/abc":     true,
		"tasks/1":          false,
		"tasks/x/main":     false,
		"tasks/1/uploads":  false,
		"tasks/1/main/sub": false,
		"sessions":         false,
		"sessions/a/b":     false,
		"exec-home":        false,
		"":                 false,
	} {
		if got := isRunDir(data, filepath.Join(data, filepath.FromSlash(rel))); got != want {
			t.Errorf("isRunDir(%q) = %v, want %v", rel, got, want)
		}
	}
	if isRunDir(data, filepath.FromSlash("/elsewhere/tasks/1/main")) {
		t.Error("a path outside the data directory is a run directory")
	}
}

// HOME and every XDG base directory point into the home.
func TestHomeEnv(t *testing.T) {
	home := filepath.FromSlash("/data/tasks/1/main/.artex-home")
	env := homeEnv(home)
	if env[0] != "HOME="+home {
		t.Fatalf("env = %v", env)
	}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(v, home) {
			t.Errorf("%s=%s is outside the home", k, v)
		}
		if !executorEnvAllowed(k) {
			t.Errorf("%s is set by the shim but missing from the allowlist", k)
		}
	}
}

// Without PrepareExecPrivilege no home is chosen, and the caller keeps its HOME.
func TestExecHomeForUnprepared(t *testing.T) {
	t.Setenv(envExecHome, "")
	if home, err := execHomeFor(t.TempDir(), 10001, 10001, true); home != "" || err != nil {
		t.Fatalf("home %q err %v, want none", home, err)
	}
}
