package server

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Agent process privilege. The server keeps running as root (the Docker image does),
// and every process the agent can start (Bash, the interactive shell, custom tools,
// stdio MCP servers) runs as the executor identity below instead. Without this a
// prompt-injected command runs as root in the same container: it reads /app/keys,
// the config file and /proc/<server-pid>/environ with root's rights.
//
// The drop is applied to the child, never to the server: SysProcAttr.Credential for
// the processes ARTEX starts itself, and for the processes the Norma library starts
// (Bash, shell, MCP stdio) a shim in the same binary that drops and then execs the
// real program. See execpriv_unix.go.

// Default executor identity. It is a numeric id, not a system account: the kernel
// accepts any uid. Override it with -exec-uid and -exec-gid.
const (
	DefaultExecUID = 10001
	DefaultExecGID = 10001
)

// execDropArg is the argv marker of the MCP drop shim:
// artex __artex-exec-drop <uid> <gid> -- <command> <args...>.
const execDropArg = "__artex-exec-drop"

// ExecPrivilege is the identity agent processes run under. The zero value is
// disabled: children keep the server's identity, which is the state before this
// change and is logged as such.
type ExecPrivilege struct {
	Enabled bool
	UID     uint32
	GID     uint32
	// MCPUID is the uid of stdio MCP servers. It differs from UID so that an agent
	// shell cannot read the environment of an MCP server (/proc/<pid>/environ needs
	// the same uid), where the secrets configured for that server are. The group
	// stays GID, so both share the run directories. Zero means the default,
	// see mcpUID.
	MCPUID uint32
	// ScriptDir is where custom-tool scripts are written for the executor to run.
	// PrepareExecPrivilege creates it and sets it: root-owned, outside the workspace
	// tree the executor can write.
	ScriptDir string
	// Range, when set, gives each task and chat session a uid of its own from it
	// while it is live (execuid.go); UID stays the identity of agent processes that
	// do not run in a run directory. Set it with WithUIDRange.
	Range ExecUIDRange
}

// ResolveExecPrivilege turns the -exec-uid and -exec-gid options into an
// ExecPrivilege. A negative uid disables the drop explicitly. explicit reports
// whether the operator set either option: an explicit drop that cannot be applied
// is an error (fail closed), while the default degrades to a single log line.
func ResolveExecPrivilege(uid, gid int, explicit bool) (ExecPrivilege, error) {
	if uid < 0 {
		logExecOnce("[exec] agent processes keep the server's identity (-exec-uid=%d): no privilege drop", uid)
		return ExecPrivilege{}, nil
	}
	if uid == 0 || gid <= 0 || uid > 1<<31-1 || gid > 1<<31-1 {
		return ExecPrivilege{}, fmt.Errorf("-exec-uid and -exec-gid must name an unprivileged id (got uid %d, gid %d; 0 is root)", uid, gid)
	}
	p := ExecPrivilege{Enabled: true, UID: uint32(uid), GID: uint32(gid)}
	if v := strings.TrimSpace(os.Getenv(MCPUIDEnv)); v != "" {
		m, err := strconv.Atoi(v)
		if err != nil || m <= 0 || m > 1<<31-1 || m == uid {
			return ExecPrivilege{}, fmt.Errorf("%s=%q must name an unprivileged uid other than -exec-uid %d", MCPUIDEnv, v, uid)
		}
		p.MCPUID = uint32(m)
	}
	if !execPrivilegeSupported() {
		return p.unavailable(explicit, "the privilege drop is not supported on this platform")
	}
	if !serverIsRoot() {
		return p.unavailable(explicit, "the server is not root, so it cannot start agent processes as another user")
	}
	return p, nil
}

// MCPUIDEnv overrides the uid stdio MCP servers run as (default: -exec-uid + 1).
const MCPUIDEnv = "ARTEX_EXEC_MCP_UID"

// mcpUID is the uid stdio MCP servers run as: MCPUID when set, otherwise the uid
// next to UID (UID+1, or UID-1 at the top of the range). It is never UID.
func (p ExecPrivilege) mcpUID() uint32 {
	if p.MCPUID != 0 && p.MCPUID != p.UID {
		return p.MCPUID
	}
	if p.UID >= 1<<31-1 {
		return p.UID - 1
	}
	return p.UID + 1
}

// unavailable answers a drop that cannot be applied: an error when the operator asked
// for it, otherwise a disabled privilege with one log line that says so.
func (p ExecPrivilege) unavailable(explicit bool, why string) (ExecPrivilege, error) {
	if explicit {
		return ExecPrivilege{}, fmt.Errorf("agent process isolation cannot be applied: %s", why)
	}
	logExecOnce("[exec] same-uid isolation unavailable: %s; agent processes run with the server's identity", why)
	return ExecPrivilege{}, nil
}

var execLogOnce sync.Map

// logExecOnce logs a message once per distinct text for the life of the process.
func logExecOnce(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if _, loaded := execLogOnce.LoadOrStore(msg, true); !loaded {
		log.Print(msg)
	}
}

// execIDString renders an executor id for the environment and the shim arguments.
func execIDString(id uint32) string { return strconv.FormatUint(uint64(id), 10) }

// WithExecPrivilege sets the identity agent processes run under. The binary passes the
// result of ResolveExecPrivilege; tests and other callers keep the zero value.
func WithExecPrivilege(p ExecPrivilege) Option {
	return func(s *Server) { s.execPriv = p }
}

// execDropCommand returns the command line that runs an MCP stdio command as the
// MCP identity (mcpUID, the executor group). The MCP stdio client starts its
// command with exec.Command and offers no credential, so the command becomes the
// server binary in drop mode (RunExecShim).
func execDropCommand(p ExecPrivilege, command string, args []string) (string, []string, error) {
	if !p.Enabled {
		return command, args, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", nil, fmt.Errorf("locate the server binary for the privilege drop: %w", err)
	}
	out := []string{execDropArg, execIDString(p.mcpUID()), execIDString(p.GID), "--", command}
	return self, append(out, args...), nil
}
