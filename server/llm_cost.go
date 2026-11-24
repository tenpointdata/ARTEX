package server

import (
	"net/http"

	"github.com/Autumn-27/artex/db"
)

// llmCost returns LLM spend and efficiency grouped by task, profile (provider),
// worker lane (role) or model over the past days (0 = all time; default 30),
// from the always-on llm_usage ledger plus archived tasks' aggregates, priced
// with the profile price lists.
func (s *Server) llmCost(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dim, err := db.ParseCostDimension(q.Get("by"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	days := atoiDefault(q.Get("days"), 30)
	resp := map[string]any{"by": dim, "days": days, "rows": []db.CostRow{}, "tasks": 0, "archived_included": 0, "archived_omitted": 0}
	pg := s.m.PG()
	if pg == nil {
		writeJSON(w, 200, resp)
		return
	}
	report, err := pg.CostBreakdown(dim, days)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	resp["rows"] = report.Rows
	resp["tasks"] = report.Tasks
	resp["archived_included"] = report.Included
	resp["archived_omitted"] = report.Omitted
	writeJSON(w, 200, resp)
}
