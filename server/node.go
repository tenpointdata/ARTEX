package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/egress"
)

// Process roles (E7, docs/DISTRIBUTED.md). One database, one control node and
// any number of worker nodes. Every role is the same binary with the same
// wiring (server.New), so a worker node runs an intent through the same guard
// hooks, intercept rules, target gate, budgets and capture proxy as the control
// node would; the role only decides which loops run in the process.
type NodeRole string

const (
	// RoleAll is the single-process deployment: API, planners and workers.
	RoleAll NodeRole = "all"
	// RoleControl serves the API and runs the planners, the deadline
	// coordinators and the schedulers; its own intent workers are optional.
	RoleControl NodeRole = "control"
	// RoleWorker serves no API. It claims intents of running tasks from the
	// database and executes them.
	RoleWorker NodeRole = "worker"
)

// ParseNodeRole accepts all, control or worker (any case); "" is all.
func ParseNodeRole(s string) (NodeRole, error) {
	switch r := NodeRole(strings.ToLower(strings.TrimSpace(s))); r {
	case "":
		return RoleAll, nil
	case RoleAll, RoleControl, RoleWorker:
		return r, nil
	default:
		return "", fmt.Errorf("unknown role %q (want all, control or worker)", s)
	}
}

// NodeConfig is this process's place in a deployment.
type NodeConfig struct {
	Role   NodeRole
	NodeID string // unique per process; worker names are "<NodeID>/work#N"
	Host   string
	// Workers is the number of intent worker loops this process runs per task.
	// -1 = the "workers" setting (the single-process default). A control node
	// with 0 leaves every intent to the worker nodes.
	Workers int
	// ControlProxy is the control node's recording proxy as worker nodes reach
	// it (for example http://artex-control:8788). Worker nodes only.
	ControlProxy string
	// ControlAPI is the control node's API as worker nodes reach it (for
	// example http://artex-control:8787); worker nodes call its node RPC for
	// the capture and evidence stores (node_rpc.go). Empty: the ControlProxy
	// host on port 8787. Worker nodes only.
	ControlAPI string
}

// Distributed reports whether other processes may run this database's intents.
func (c NodeConfig) Distributed() bool { return c.Role == RoleControl || c.Role == RoleWorker }

// workerName is the claim owner of worker loop i on this node. Local names keep
// the single-process form so nothing changes for RoleAll and RoleControl.
func (c NodeConfig) workerName(i int) string {
	if c.Role == RoleWorker {
		return fmt.Sprintf("%s/work#%d", c.NodeID, i)
	}
	return fmt.Sprintf("work#%d", i)
}

// remoteOwner reports whether a claim owner is a worker node's loop.
func remoteOwner(owner string) bool { return strings.Contains(owner, "/work#") }

// NodeConfigFromEnv reads ARTEX_NODE_ID (default: host name), ARTEX_LOCAL_WORKERS
// (control) or ARTEX_NODE_WORKERS (worker), ARTEX_CONTROL_PROXY and
// ARTEX_CONTROL_API.
func NodeConfigFromEnv(role NodeRole) (NodeConfig, error) {
	host, _ := os.Hostname()
	c := NodeConfig{Role: role, Host: host, Workers: -1, NodeID: strings.TrimSpace(os.Getenv("ARTEX_NODE_ID"))}
	if c.NodeID == "" {
		c.NodeID = host
	}
	if c.NodeID == "" || strings.ContainsAny(c.NodeID, "/ \t") {
		return c, fmt.Errorf("ARTEX_NODE_ID %q must be non-empty and contain no '/' or spaces", c.NodeID)
	}
	key := map[NodeRole]string{RoleControl: "ARTEX_LOCAL_WORKERS", RoleWorker: "ARTEX_NODE_WORKERS"}[role]
	if v := strings.TrimSpace(os.Getenv(key)); key != "" && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 64 {
			return c, fmt.Errorf("%s=%q: want an integer 0-64", key, v)
		}
		c.Workers = n
	}
	if role == RoleWorker {
		if c.Workers < 0 {
			c.Workers = defaultWorkers
		}
		if c.Workers == 0 {
			return c, fmt.Errorf("ARTEX_NODE_WORKERS=0: a worker node needs at least one worker")
		}
		c.ControlProxy = strings.TrimSpace(os.Getenv("ARTEX_CONTROL_PROXY"))
		c.ControlAPI = strings.TrimSpace(os.Getenv("ARTEX_CONTROL_API"))
	}
	return c, nil
}

// WithNodeConfig sets the process role. Without it the server is RoleAll.
func WithNodeConfig(c NodeConfig) Option { return func(s *Server) { s.node = c } }

// localWorkers is the worker loop count per task in this process.
func (c NodeConfig) localWorkers(setting func() int) int {
	if c.Workers >= 0 {
		return c.Workers
	}
	return setting()
}

// settingDistributedBudgets is where the control node publishes its run budget
// configuration (ARTEX_*_BUDGET_*), so every worker node meters the same
// budget no matter what its own environment says.
const settingDistributedBudgets = "distributed_run_budgets"

type publishedBudgets struct {
	Task   agent.Budget `json:"task"`
	Intent agent.Budget `json:"intent"`
}

// publishBudgets stores the control node's budget configuration.
func publishBudgets(pg *db.DB) error {
	task, intent := agent.BudgetsFromEnv()
	b, err := json.Marshal(publishedBudgets{Task: task, Intent: intent})
	if err != nil {
		return err
	}
	return pg.SetSetting(settingDistributedBudgets, string(b))
}

// readPublishedBudgets returns the control node's budgets; ok is false when
// none were published or they cannot be read (a worker node then claims
// nothing: an unknown budget is not an absent one).
func readPublishedBudgets(pg *db.DB) (task, intent agent.Budget, ok bool) {
	v, found, err := pg.GetSetting(settingDistributedBudgets)
	if err != nil || !found {
		return task, intent, false
	}
	var b publishedBudgets
	if json.Unmarshal([]byte(v), &b) != nil {
		return task, intent, false
	}
	return b.Task, b.Intent, true
}

// runNodeHeartbeat keeps this node's registry row fresh and rescales the noise
// governor to this node's share of the live nodes (each node paces the same
// targets independently, so together they stay within one profile). It also
// follows a noise profile change made through the control node's API.
func (s *Server) runNodeHeartbeat(ctx context.Context) {
	pg := s.m.pg
	row := db.WorkerNode{NodeID: s.node.NodeID, Role: string(s.node.Role), Host: s.node.Host, Version: BuildVersion,
		Workers: s.node.localWorkers(s.m.Workers)}
	beat := func(first bool) {
		if err := pg.HeartbeatNode(row, first); err != nil {
			log.Printf("[node %s] heartbeat: %v", row.NodeID, err)
			return
		}
		n, err := pg.LiveNodes()
		if err != nil {
			return
		}
		base := startupNoiseProfile(pg)
		if want := egress.Share(base, n); egress.CurrentProfile() != want {
			egress.SetProfile(want)
			log.Printf("[node %s] %d live node(s): noise profile %s at 1/%d", row.NodeID, n, base.Name, n)
		}
	}
	beat(true)
	t := time.NewTicker(db.NodeHeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = pg.RemoveNode(row.NodeID)
			return
		case <-t.C:
			beat(false)
		}
	}
}

// listNodes reports the processes sharing this database (GET /api/settings/nodes).
func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.m.pg.ListNodes()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"self": s.node.NodeID, "role": s.node.Role, "nodes": nodes})
}
