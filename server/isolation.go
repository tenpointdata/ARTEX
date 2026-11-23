package server

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Startup self-test of agent isolation. Before the stores open, the server checks
// that what an agent can run really is isolated from it: IsolationSelfTest
// (isolation_unix.go) starts a probe through the Bash shim, the way the agent's Bash
// starts, and checks from inside it that it runs as the executor, cannot read the
// server's secrets or its process, sees only the allowlisted environment and cannot
// open a database session without the server's password; it also checks that the
// file tools cannot read the secrets. EnforceIsolation then decides the start. It
// fails closed: no agent runs unsandboxed unless the operator said so.
//
//   - The self-test passes: the server starts isolated.
//   - Linux with the privilege drop on (the server is root, as in the Docker image):
//     isolation is expected there and the self-test shows it is broken, so the start
//     stops. The development escape -dev-insecure-no-isolation, off by default, lets
//     it go on degraded.
//   - Anywhere else (not Linux, the server is not root, or -exec-uid=-1): isolation
//     cannot be built on that host, and agent processes would run with the server's
//     own user and rights. The start stops unless the operator opts in with
//     ARTEX_ALLOW_UNSANDBOXED=1 (or the development escape). A non-root systemd
//     unit, dev.sh, Windows and macOS need that opt-in.
//
// A degraded start logs a warning banner that says what is exposed, and reports the
// state in GET /api/health to a signed-in caller, which the web UI shows as a
// banner on every page.

// DevNoIsolationFlag is the command-line flag of the development escape.
const DevNoIsolationFlag = "dev-insecure-no-isolation"

// AllowUnsandboxedEnv is the environment variable an operator sets to 1 to start a
// server whose agent processes cannot be isolated from it (see EnforceIsolation).
const AllowUnsandboxedEnv = "ARTEX_ALLOW_UNSANDBOXED"

// unsandboxedOptIn reports whether the operator set AllowUnsandboxedEnv to 1, true
// or yes.
func unsandboxedOptIn() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(AllowUnsandboxedEnv))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// The isolation modes the server reports.
const (
	IsolationIsolated = "isolated"
	IsolationDegraded = "degraded"
)

// IsolationStatus is the agent isolation the server runs with.
type IsolationStatus struct {
	Mode string `json:"mode"`
	// Reason is the self-test failure of a degraded start.
	Reason string `json:"reason,omitempty"`
}

// WithIsolationStatus records the result of EnforceIsolation for GET /api/health.
func WithIsolationStatus(st IsolationStatus) Option {
	return func(s *Server) { s.isolation = st }
}

// IsolationTargets are the server's paths and database the self-test checks the
// executor cannot reach.
type IsolationTargets struct {
	DataDir    string
	KeyDir     string
	ConfigPath string
	SkillDir   string
	// DSN is the server's database DSN; only its hosts, ports, user and database
	// name reach the probe, never its password.
	DSN string
}

// EnforceIsolation decides the start from the self-test result, the platform goos
// and the privilege drop p (see the rules above). It returns the status to report,
// or an error when the start must stop.
func EnforceIsolation(goos string, p ExecPrivilege, selfTest error, devEscape bool) (IsolationStatus, error) {
	if selfTest == nil {
		log.Printf("[isolation] self-test passed: agent processes and file tools run as the executor and cannot reach the server's secrets")
		if devEscape {
			log.Printf("[isolation] -%s is set and not needed: the self-test passed", DevNoIsolationFlag)
		}
		return IsolationStatus{Mode: IsolationIsolated}, nil
	}
	required := isolationRequired(goos, p)
	if required && !devEscape {
		return IsolationStatus{}, fmt.Errorf("agent isolation self-test failed: %v. A Linux server running as root with the privilege drop does not start without agent isolation: fix the cause above, or pass -%s for development only", selfTest, DevNoIsolationFlag)
	}
	if !required && !devEscape && !unsandboxedOptIn() {
		return IsolationStatus{}, fmt.Errorf("agent isolation is unavailable on this host: %v. "+
			"Agent processes (Bash, the shell, custom tools, MCP stdio servers) would run unsandboxed with this server's own user and rights: "+
			"a prompt-injected agent could read the JWT and secret keys, the config file and the database password, change the database "+
			"(its own approval rules included) and replace the artex binary that start.sh relaunches. Refusing to start. "+
			"Run the Docker image (Linux, the server as root with the privilege drop) for isolation, or set %s=1 to accept running unsandboxed",
			selfTest, AllowUnsandboxedEnv)
	}
	logDegradedBanner(selfTest, required)
	return IsolationStatus{Mode: IsolationDegraded, Reason: selfTest.Error()}, nil
}

// isolationRequired reports whether the host can isolate agents, so that a failed
// self-test means broken isolation: Linux, with the privilege drop on (root).
func isolationRequired(goos string, p ExecPrivilege) bool {
	return goos == "linux" && p.Enabled
}

// degradedBanner is the warning a degraded start logs. escaped is set when the start
// went on only through the development escape. It names what an agent process can
// reach: everything the server's own user can.
func degradedBanner(cause error, escaped bool) []string {
	bar := strings.Repeat("!", 78)
	why := []string{
		"!! This host cannot isolate agents: that needs Linux and a root server with the",
		"!! privilege drop (-exec-uid), as in the Docker image. The operator opted in",
		"!! with " + AllowUnsandboxedEnv + "=1 (or -" + DevNoIsolationFlag + ").",
	}
	if escaped {
		why = []string{"!! -" + DevNoIsolationFlag + " is set. DEVELOPMENT ONLY."}
	}
	lines := append([]string{bar, "!! AGENT ISOLATION IS DEGRADED: AGENT PROCESSES RUN UNSANDBOXED."}, why...)
	return append(lines,
		"!! Self-test failure: "+cause.Error(),
		fmt.Sprintf("!! Agent shells, custom tools and MCP stdio servers may run as this server's user (uid %d)", os.Getuid()),
		"!! with its rights. A prompt-injected agent can then:",
		"!!  - read the server's keys (jwt.key: forge any session; secret.key: decrypt the",
		"!!    stored credentials), its config, its database credentials and the LLM API keys;",
		"!!  - connect to the database and change anything in it, its own approval rules for",
		"!!    commands, custom tools, MCP servers and skills included;",
		"!!  - replace the artex binary, start.sh, skills and tools on disk (start.sh refuses",
		"!!    to relaunch a binary that changed while it ran; the next manual start runs it);",
		"!!  - read, write and signal everything else this user can (home directory, SSH keys,",
		"!!    other processes of this user).",
		"!! Do not use this mode on a server that holds real targets or credentials.",
		bar,
	)
}

func logDegradedBanner(cause error, escaped bool) {
	for _, line := range degradedBanner(cause, escaped) {
		log.Print("[isolation] " + line)
	}
}
