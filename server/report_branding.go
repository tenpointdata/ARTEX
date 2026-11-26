package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Autumn-27/artex/report"
)

// settingReportBranding holds the JSON report.Branding every branded export
// uses by default, so the operator sets it once instead of per download.
const settingReportBranding = "report.branding"

// reportBranding reads the stored branding; unset means the defaults.
func (s *Server) reportBranding() (report.Branding, error) {
	var b report.Branding
	raw, ok, err := s.m.pg.GetSetting(settingReportBranding)
	if err != nil || !ok {
		return b, err
	}
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return report.Branding{}, err
	}
	return b, nil
}

// exportBranding is the stored branding with the per-export overrides an
// engagement needs most (client, title) taken from the query string.
func (s *Server) exportBranding(r *http.Request) (report.Branding, error) {
	b, err := s.reportBranding()
	if err != nil {
		return b, err
	}
	q := r.URL.Query()
	if v := q.Get("client"); v != "" {
		b.Client = v
	}
	if v := q.Get("title"); v != "" {
		b.Title = v
	}
	return b.Normalize()
}

func (s *Server) getReportBranding(w http.ResponseWriter, r *http.Request) {
	b, err := s.reportBranding()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, b)
}

// putReportBranding replaces the stored branding (whole object; "" clears a field).
func (s *Server) putReportBranding(w http.ResponseWriter, r *http.Request) {
	var b report.Branding
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeErr(w, 400, "bad json: "+err.Error())
		return
	}
	b, err := b.Normalize()
	if errors.Is(err, report.ErrInvalidBranding) {
		writeErr(w, 400, err.Error())
		return
	}
	raw, _ := json.Marshal(b)
	if err := s.m.pg.SetSetting(settingReportBranding, string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, b)
}
