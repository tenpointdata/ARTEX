package server

import (
	"net/http"

	"github.com/Autumn-27/artex/methodology"
)

// methodologySummaryDTO is one built-in methodology as offered on the task
// create form: enough to choose it, without its item list.
type methodologySummaryDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Source      string   `json:"source"`
	AppliesWhen []string `json:"applies_when"`
	ItemCount   int      `json:"item_count"`
}

// listMethodologies returns the built-in methodologies a task can be checked
// against (GET /api/methodologies), sorted by id.
func (s *Server) listMethodologies(w http.ResponseWriter, r *http.Request) {
	all, err := methodology.All()
	if err != nil {
		writeErr(w, 500, "methodologies unavailable: "+err.Error())
		return
	}
	out := make([]methodologySummaryDTO, 0, len(all))
	for _, m := range all {
		out = append(out, methodologySummaryDTO{
			ID: m.ID, Name: m.Name, Version: m.Version, Source: m.Source,
			AppliesWhen: append([]string{}, m.AppliesWhen...), ItemCount: len(m.Items),
		})
	}
	writeJSON(w, 200, out)
}
