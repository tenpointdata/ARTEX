package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
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
	ws.followAgentSettings(&f)
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

	// An unchanged revision reads one row and changes nothing, even when a row
	// is edited behind the API's back.
	if err := control.pg.SetBool(settingTrafficCapture, false); err != nil {
		t.Fatal(err)
	}
	ws.followAgentSettings(&f)
	if !wm.TrafficEnabled() {
		t.Fatal("reloaded without a revision bump")
	}
	if _, err := control.pg.BumpSettingsRevision(db.AgentSettingsRevisionKey); err != nil {
		t.Fatal(err)
	}
	ws.followAgentSettings(&f)
	if wm.TrafficEnabled() {
		t.Fatal("worker node did not follow capture off")
	}
}
