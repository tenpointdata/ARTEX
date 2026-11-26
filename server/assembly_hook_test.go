package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// fakeMCPEnv, set to 1 in a child's environment, makes the test binary a stdio
// MCP server with one tool, "ping" (runFakeMCPIfRequested).
const fakeMCPEnv = "ARTEX_TEST_FAKE_MCP"

// runFakeMCPIfRequested serves the MCP handshake and tools/list on stdio
// (newline-delimited JSON-RPC) and exits when stdin closes.
func runFakeMCPIfRequested() {
	if os.Getenv(fakeMCPEnv) != "1" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // a notification
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "fake", "version": "0"}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "ping", "description": "ping", "inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}
	os.Exit(0)
}

// countingCloser counts its Close calls.
type countingCloser struct{ n atomic.Int32 }

func (c *countingCloser) Close() error { c.n.Add(1); return nil }

// toolsOf returns the names of tools that belong to MCP server srv.
func toolsOf(tools []actool.CoreTool, srv string) []string {
	var out []string
	for _, t := range tools {
		if strings.Contains(t.Name(), srv) {
			out = append(out, t.Name())
		}
	}
	return out
}

// wireAgentAugment's prepareMCP hook, against Postgres and real stdio spawns:
//   - a hook error skips the server, even one that would start;
//   - the server spawned is the one the hook returned, and its closer is
//     released by the run's cleanup, not before;
//   - a closer whose server fails to start is released at once, not held
//     until the run ends.
func TestWireAgentAugmentPrepareMCPHook(t *testing.T) {
	dsn, _, err := db.DSN()
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	pg, err := db.Open(dsn)
	if err != nil {
		db.SkipPGUnavailable(t, err)
	}
	t.Cleanup(func() { _ = pg.Close() })                // after the row cleanups below, which run first
	if err := pg.InitSecrets(t.TempDir()); err != nil { // MCP env is a secret column
		t.Fatal(err)
	}
	ag, _ := pg.GetAgentByKey("planner")
	if ag == nil {
		t.Fatal("planner agent missing")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fakeEnv, _ := json.Marshal(map[string]string{fakeMCPEnv: "1"})
	// Stored without the env that makes the binary an MCP server: only a
	// spawn of what the hook returned (with the env) can list tools.
	add := func(name, command string) {
		if all, err := pg.ListMCP(); err == nil { // a row an interrupted run left
			for _, m := range all {
				if m.Name == name {
					_ = pg.DeleteMCP(m.ID)
				}
			}
		}
		id, err := pg.SaveMCP(&db.MCPServer{Name: name, Transport: "stdio", Command: command, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pg.DeleteMCP(id) })
		if err := pg.ToggleVisibility(ag.ID, "mcp", id, true); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pg.ToggleVisibility(ag.ID, "mcp", id, false) })
	}
	add("thookerr", self)
	add("thookok", self)
	add("thookdead", self+"-missing")

	okCloser, deadCloser := &countingCloser{}, &countingCloser{}
	var errCalls atomic.Int32
	prepare := func(_ context.Context, srv *db.MCPServer) (*db.MCPServer, io.Closer, error) {
		out := *srv
		out.Env = fakeEnv
		switch srv.Name {
		case "thookerr":
			errCalls.Add(1)
			return nil, nil, errors.New("relay refused")
		case "thookok":
			return &out, okCloser, nil
		case "thookdead":
			return &out, deadCloser, nil
		}
		// Any other server visible to the planner in a shared database is
		// skipped too, so the test never spawns it.
		return nil, nil, fmt.Errorf("not under test: %s", srv.Name)
	}
	wireAgentAugment(pg, t.TempDir(), nil, ExecPrivilege{}, prepare)
	t.Cleanup(func() { agent.SetToolAugment(nil) })

	extra, _, cleanup := agent.ToolAugment()(context.Background(), "planner")
	if cleanup == nil {
		t.Fatal("no cleanup returned")
	}
	if errCalls.Load() != 1 {
		t.Fatalf("hook called %d times for the erroring server, want 1", errCalls.Load())
	}
	if got := toolsOf(extra, "thookerr"); len(got) != 0 {
		t.Fatalf("a server whose hook failed was started: %v", got)
	}
	if got := toolsOf(extra, "thookok"); len(got) != 1 {
		t.Fatalf("tools of the prepared server: %v, want its one tool", got)
	}
	if n := deadCloser.n.Load(); n != 1 {
		t.Fatalf("closer of a server that failed to start closed %d times before cleanup, want 1", n)
	}
	if n := okCloser.n.Load(); n != 0 {
		t.Fatalf("closer of a running server closed %d times before cleanup", n)
	}
	cleanup()
	if n := okCloser.n.Load(); n != 1 {
		t.Fatalf("cleanup closed the running server's closer %d times, want 1", n)
	}
	if n := deadCloser.n.Load(); n != 1 {
		t.Fatalf("cleanup closed the failed server's closer again (%d)", n)
	}
}
