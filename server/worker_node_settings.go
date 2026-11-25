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
//     and the scope it applies.
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
	rebuild := settingsMoved && s.m.loadAgentSettings()
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
