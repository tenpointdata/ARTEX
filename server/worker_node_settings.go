package server

import (
	"log"

	"github.com/Autumn-27/artex/db"
)

// agentSettingsFollower is what a worker node last applied (followAgentSettings).
// Touched by the dispatch loop only.
type agentSettingsFollower struct {
	llmRev   int64
	agentRev int64
	seen     bool
}

// followAgentSettings is the one place a worker node picks up the settings the
// control node bakes into agents. It runs at the start of every dispatch poll
// and returns why claims must wait this poll ("" = they need not):
//   - agent settings (Manager.loadAgentSettings): traffic capture, the global
//     egress proxy and web search (backend, keys, proxy). The values themselves
//     are compared every poll, not a revision, so a change is followed even when
//     nothing announced it (a failed bump, a direct write). Without this, turning
//     capture on would leave worker agents connecting directly, past the
//     recording proxy and the scope it applies.
//     The LLM recording switch is reloaded with them (no rebuild needed).
//   - agent config (db.AgentConfigRevisionKey, c-6bcdd3): an agent's saved
//     max turns, run seconds and web search flag. The task agents are dropped.
//   - LLM configuration (db.LLMConfigRevisionKey, c-1d3e0a): an LLM profile
//     saved, activated or deleted, the retry policy, the failover switches or an
//     agent's profile binding. The node drops its provider caches and re-applies
//     the active profile, as the control node's API handler does.
//
// Fail closed: while either cannot be read, claims are blocked and the current
// agents are kept. When the agent settings change after the first poll, the
// task agents are dropped and claims wait one poll, so no intent starts on the
// old egress path. An intent already running keeps the bundle it started with.
func (s *Server) followAgentSettings(f *agentSettingsFollower) string {
	settingsMoved, err := s.m.loadAgentSettings()
	if err != nil {
		log.Printf("[node %s] agent settings unreadable; claims blocked, current agents kept: %v", s.node.NodeID, err)
		return "agent settings (capture/proxy/web search) unreadable"
	}
	llmRev, err := s.m.pg.SettingsRevision(db.LLMConfigRevisionKey)
	if err != nil {
		log.Printf("[node %s] LLM config revision unreadable; claims blocked, current agents kept: %v", s.node.NodeID, err)
		return "LLM config revision unreadable"
	}
	agentRev, err := s.m.pg.SettingsRevision(db.AgentConfigRevisionKey)
	if err != nil {
		log.Printf("[node %s] agent config revision unreadable; claims blocked, current agents kept: %v", s.node.NodeID, err)
		return "agent config revision unreadable"
	}
	first := !f.seen
	llmMoved := first || llmRev != f.llmRev
	if agentRev != f.agentRev {
		// An agent's saved settings (max turns, run seconds, web search) are
		// read when a task's agents are built: drop them so the next claim
		// reads the saved row (c-6bcdd3).
		f.agentRev = agentRev
		s.invalidateTaskAgents()
	}
	if !settingsMoved && !llmMoved {
		return ""
	}
	f.llmRev, f.seen = llmRev, true
	rebuild := settingsMoved
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
	if !rebuild {
		return ""
	}
	block := ""
	if settingsMoved && !first {
		log.Printf("[node %s] capture/proxy/web-search settings changed on the control node; rebuilding task agents", s.node.NodeID)
		block = "shared capture/proxy settings changed: rebuilding agents"
	}
	if !on {
		s.invalidateTaskAgents() // no global agents to build; task agents are rebuilt on demand
		return block
	}
	if err := s.applyLLM(cfg); err != nil {
		// applyLLM failed before it dropped anything. Task agents are built per
		// task from the Manager's (already updated) settings, so dropping them is
		// enough for the egress path; the LLM half is retried next poll.
		s.invalidateTaskAgents()
		f.seen = false
		log.Printf("[node %s] rebuild agents after a settings change: %v", s.node.NodeID, err)
		return block
	}
	log.Printf("[node %s] agent settings applied, LLM config revision %d (%s / %s, traffic capture %v)",
		s.node.NodeID, llmRev, cfg.Provider(), cfg.Model, s.m.TrafficEnabled())
	return block
}

// bumpAgentConfigRevision tells worker nodes that an agent's saved settings
// changed (followAgentSettings). Callers write the database first. A worker
// node never bumps it: it would follow its own bump.
func (s *Server) bumpAgentConfigRevision() {
	if s.node.Role == RoleWorker || s.m == nil || s.m.pg == nil {
		return
	}
	if _, err := s.m.pg.BumpSettingsRevision(db.AgentConfigRevisionKey); err != nil {
		log.Printf("[agents] agent config revision not bumped; worker nodes keep their task agents until restart: %v", err)
	}
}
