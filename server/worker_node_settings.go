package server

import (
	"log"

	"github.com/Autumn-27/artex/db"
)

// agentSettingsFollower tracks the revisions a worker node last applied
// (followAgentSettings): the agent settings and the LLM configuration.
type agentSettingsFollower struct {
	rev    int64
	llmRev int64
	seen   bool
}

// followAgentSettings makes a worker node pick up a change the control node
// saved. Two revisions are followed:
//   - agent settings (db.AgentSettingsRevisionKey): traffic capture, the global
//     egress proxy and web search are baked into the agents when they are
//     built, and a worker node builds its own. Without it, turning capture on
//     would leave worker agents connecting directly, past the recording proxy
//     and the scope it applies. The same revision carries the LLM recording
//     switch (held in memory) and an agent's saved settings (max turns, run
//     seconds, web search: read when a task bundle is built, c-6bcdd3).
//   - LLM configuration (db.LLMConfigRevisionKey, c-1d3e0a): an LLM profile
//     saved, activated or deleted, the retry policy, the failover switches or
//     an agent's profile binding. The node drops its provider caches and
//     re-applies the active profile, as the control node's API handler does,
//     so the next intent it claims runs on the new configuration.
//
// Called on every dispatch poll; it reads two rows unless a revision moved. The
// first call always reloads, so a bump between start-up and the first poll is
// not lost. An intent already running keeps the bundle it started with.
func (s *Server) followAgentSettings(f *agentSettingsFollower) {
	pg := s.m.pg
	rev, err := pg.SettingsRevision(db.AgentSettingsRevisionKey)
	if err != nil {
		log.Printf("[node %s] agent settings revision unreadable; keeping the current agents: %v", s.node.NodeID, err)
		return
	}
	llmRev, err := pg.SettingsRevision(db.LLMConfigRevisionKey)
	if err != nil {
		log.Printf("[node %s] LLM config revision unreadable; keeping the current agents: %v", s.node.NodeID, err)
		return
	}
	settingsMoved := !f.seen || rev != f.rev
	llmMoved := !f.seen || llmRev != f.llmRev
	if !settingsMoved && !llmMoved {
		return
	}
	f.rev, f.llmRev, f.seen = rev, llmRev, true
	rebuild := false
	if settingsMoved {
		rebuild = s.m.loadAgentSettings()
		// An agent's own settings (max turns, run seconds, web search) are
		// read when a task bundle is built; drop the bundles so the next claim
		// reads the saved row.
		s.invalidateTaskAgents()
	}
	s.cfgMu.Lock()
	cfg, on := s.llmCfg, s.llmOn
	s.cfgMu.Unlock()
	if llmMoved {
		s.applyRetryPolicy()
		s.dropProfileCaches()
		if active, ok := s.loadLLMConfig(); ok {
			cfg, on, rebuild = active, true, true
		}
	}
	if !rebuild || !on {
		return
	}
	if err := s.applyLLM(cfg); err != nil {
		log.Printf("[node %s] rebuild agents after a settings change: %v", s.node.NodeID, err)
		return
	}
	log.Printf("[node %s] agent settings revision %d, LLM config revision %d applied (%s / %s, traffic capture %v)",
		s.node.NodeID, rev, llmRev, cfg.Provider(), cfg.Model, s.m.TrafficEnabled())
}

// bumpAgentSettingsRevision tells worker nodes that a setting they bake into
// agents or hold in memory changed (followAgentSettings). Callers write the
// database first. A worker node never bumps it: it would follow its own bump.
func (s *Server) bumpAgentSettingsRevision() {
	if s.node.Role == RoleWorker || s.m == nil || s.m.pg == nil {
		return
	}
	if _, err := s.m.pg.BumpSettingsRevision(db.AgentSettingsRevisionKey); err != nil {
		log.Printf("[settings] agent settings revision not bumped; worker nodes keep their agents until restart: %v", err)
	}
}
