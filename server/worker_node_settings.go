package server

import (
	"log"

	"github.com/Autumn-27/artex/db"
)

// agentSettingsFollower tracks the agent settings revision a worker node last
// applied (followAgentSettings).
type agentSettingsFollower struct {
	rev  int64
	seen bool
}

// followAgentSettings makes a worker node pick up a settings change the control
// node saved: traffic capture, the global egress proxy and web search are baked
// into the agents when they are built, and a worker node builds its own. Without
// it, turning capture on would leave worker agents connecting directly, past the
// recording proxy and the scope it applies. Called on every dispatch poll; it
// reads one row unless the revision moved. The first call always reloads, so a
// bump between start-up and the first poll is not lost.
func (s *Server) followAgentSettings(f *agentSettingsFollower) {
	rev, err := s.m.pg.SettingsRevision(db.AgentSettingsRevisionKey)
	if err != nil {
		log.Printf("[node %s] agent settings revision unreadable; keeping the current agents: %v", s.node.NodeID, err)
		return
	}
	if f.seen && rev == f.rev {
		return
	}
	f.rev, f.seen = rev, true
	if !s.m.loadAgentSettings() {
		return
	}
	s.cfgMu.Lock()
	cfg, on := s.llmCfg, s.llmOn
	s.cfgMu.Unlock()
	if !on {
		return
	}
	if err := s.applyLLM(cfg); err != nil {
		log.Printf("[node %s] rebuild agents after a settings change: %v", s.node.NodeID, err)
		return
	}
	log.Printf("[node %s] agent settings revision %d applied (traffic capture %v)", s.node.NodeID, rev, s.m.TrafficEnabled())
}
