package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A worker node builds its own agents from the settings rows, so a capture
// toggle saved on the control node must reach it: otherwise its agents keep
// connecting directly, past the recording proxy and the scope it applies.
func TestWorkerNodeFollowsAgentSettings(t *testing.T) {
	skipWithoutDSN(t)
	control, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	if err := control.SetTrafficEnabled(false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.pg.SetBool(settingTrafficCapture, false) })

	wm, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wm.Close() })
	ws := &Server{m: wm, node: NodeConfig{Role: RoleWorker, NodeID: "node-s"}}
	var f agentSettingsFollower
	if block := ws.followAgentSettings(&f); block != "" {
		t.Fatalf("first poll blocked: %q", block)
	}
	if wm.TrafficEnabled() {
		t.Fatal("capture on before any change")
	}

	// The control node saves the toggle through the settings API.
	cs := &Server{m: control, node: NodeConfig{Role: RoleControl, NodeID: "ctl"}}
	rec := httptest.NewRecorder()
	cs.putSettings(rec, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"traffic_capture":true}`)))
	if rec.Code != 200 {
		t.Fatalf("putSettings: %d %s", rec.Code, rec.Body.String())
	}
	if !control.TrafficEnabled() {
		t.Fatal("control node did not apply the toggle")
	}
	ws.followAgentSettings(&f)
	if !wm.TrafficEnabled() {
		t.Fatal("worker node kept capture off after the control node turned it on")
	}

	// A row edited behind the API's back (no revision, no announcement) is
	// followed too: the values are compared, so a lost signal cannot leave a
	// worker's agents on the old egress path.
	if err := control.pg.SetBool(settingTrafficCapture, false); err != nil {
		t.Fatal(err)
	}
	if block := ws.followAgentSettings(&f); block == "" {
		t.Fatal("claims not held for the poll that changed the egress settings")
	}
	if wm.TrafficEnabled() {
		t.Fatal("worker node did not follow capture off")
	}
	if block := ws.followAgentSettings(&f); block != "" {
		t.Fatalf("claims still blocked with nothing changed: %q", block)
	}
}

// Fail closed: while the settings cannot be read, claims are blocked and the
// values the node runs on are kept.
func TestWorkerNodeBlocksClaimsWhileAgentSettingsUnreadable(t *testing.T) {
	skipWithoutDSN(t)
	wm, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	ws := &Server{m: wm, node: NodeConfig{Role: RoleWorker, NodeID: "node-u"}}
	var f agentSettingsFollower
	if block := ws.followAgentSettings(&f); block != "" {
		t.Fatalf("first poll blocked: %q", block)
	}
	was := wm.TrafficEnabled()
	_ = wm.Close() // every read now fails
	if block := ws.followAgentSettings(&f); block == "" {
		t.Fatal("claims allowed while the agent settings are unreadable")
	}
	if wm.TrafficEnabled() != was {
		t.Fatal("an unreadable read changed the applied settings")
	}
}
