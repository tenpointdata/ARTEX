package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/notify"
	"github.com/Autumn-27/artex/ticketing"
)

// HTTP API for ticket trackers (E2). Mounted behind requireAuth like the
// notification API it mirrors: configs are masked on readback with the
// tracker's SecretKeys, and changing a destination without re-entering the
// credentials is refused (notify.PrepareConfigUpdateKeys).

// trackerOptionKeys are server-side keys a tracker config may carry next to
// the adapter's own.
var trackerOptionKeys = []string{"include_evidence"}

type ticketTrackerDTO struct {
	ID         int64          `json:"id"`
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Enabled    bool           `json:"enabled"`
	Config     map[string]any `json:"config"`
	SecretKeys []string       `json:"secret_keys"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func trackerConfig(t *db.TicketTracker) map[string]any {
	cfg := map[string]any{}
	if len(t.Config) > 0 {
		_ = json.Unmarshal(t.Config, &cfg)
	}
	return cfg
}

func toTicketTrackerDTO(t *db.TicketTracker) ticketTrackerDTO {
	dto := ticketTrackerDTO{ID: t.ID, Name: t.Name, Kind: t.Kind, Enabled: t.Enabled, Config: map[string]any{},
		SecretKeys: []string{}, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
	if impl, ok := ticketing.Get(t.Kind); ok {
		dto.SecretKeys = impl.SecretKeys()
		dto.Config = notify.MaskConfigKeys(impl.SecretKeys(), trackerConfig(t))
	}
	return dto
}

func (s *Server) trackersMeta(w http.ResponseWriter, _ *http.Request) {
	kinds := make([]map[string]any, 0)
	for _, k := range ticketing.Kinds() {
		impl, _ := ticketing.Get(k)
		kinds = append(kinds, map[string]any{"kind": k, "secret_keys": impl.SecretKeys(), "destination_keys": impl.DestinationKeys()})
	}
	writeJSON(w, 200, map[string]any{"kinds": kinds, "option_keys": trackerOptionKeys})
}

func (s *Server) listTrackers(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	list, err := pg.ListTicketTrackers(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]ticketTrackerDTO, 0, len(list))
	for _, t := range list {
		out = append(out, toTicketTrackerDTO(t))
	}
	writeJSON(w, 200, map[string]any{"trackers": out})
}

type ticketTrackerRequest struct {
	Name    *string        `json:"name"`
	Kind    *string        `json:"kind"`
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (s *Server) createTracker(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req ticketTrackerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, 400, "request body is not valid JSON")
		return
	}
	if req.Kind == nil {
		writeErr(w, 400, "kind is required: "+strings.Join(ticketing.Kinds(), " / "))
		return
	}
	impl, ok := ticketing.Get(*req.Kind)
	if !ok {
		writeErr(w, 400, "invalid tracker kind, available: "+strings.Join(ticketing.Kinds(), " / "))
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}
	if err := impl.Validate(req.Config); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	t := &db.TicketTracker{Kind: *req.Kind, Enabled: req.Enabled == nil || *req.Enabled}
	if req.Name != nil {
		t.Name = strings.TrimSpace(*req.Name)
	}
	if t.Name == "" {
		t.Name = *req.Kind
	}
	raw, _ := json.Marshal(req.Config)
	t.Config = raw
	id, err := pg.SaveTicketTracker(r.Context(), t)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := pg.TicketTrackerByID(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, toTicketTrackerDTO(saved))
}

// trackerFromPath loads {id}; it writes the error response and returns nil on failure.
func (s *Server) trackerFromPath(w http.ResponseWriter, r *http.Request, pg *db.DB) *db.TicketTracker {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "bad tracker id")
		return nil
	}
	t, err := pg.TicketTrackerByID(r.Context(), id)
	if errors.Is(err, db.ErrTicketTrackerNotFound) {
		writeErr(w, 404, err.Error())
		return nil
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return nil
	}
	return t
}

func (s *Server) updateTracker(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	t := s.trackerFromPath(w, r, pg)
	if t == nil {
		return
	}
	var req ticketTrackerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, 400, "request body is not valid JSON")
		return
	}
	if req.Kind != nil && *req.Kind != t.Kind {
		writeErr(w, 400, "a tracker's kind cannot change; create a new tracker")
		return
	}
	impl, ok := ticketing.Get(t.Kind)
	if !ok {
		writeErr(w, 400, "tracker kind is no longer supported")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		t.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	if req.Config != nil {
		merged, err := notify.PrepareConfigUpdateKeys(impl.SecretKeys(), impl.DestinationKeys(), trackerConfig(t), req.Config)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if err := impl.Validate(merged); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		t.Config, _ = json.Marshal(merged)
	}
	if _, err := pg.SaveTicketTracker(r.Context(), t); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := pg.TicketTrackerByID(r.Context(), t.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, toTicketTrackerDTO(saved))
}

func (s *Server) deleteTracker(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	t := s.trackerFromPath(w, r, pg)
	if t == nil {
		return
	}
	if err := pg.DeleteTicketTracker(r.Context(), t.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": t.ID})
}

// enabledSyncer loads the tracker {id} and builds its syncer, refusing a disabled one.
func (s *Server) enabledSyncer(w http.ResponseWriter, r *http.Request, pg *db.DB) *ticketSyncer {
	t := s.trackerFromPath(w, r, pg)
	if t == nil {
		return nil
	}
	if !t.Enabled {
		writeErr(w, 409, "tracker is disabled")
		return nil
	}
	baseURL, _, _ := pg.GetSetting(settingNotifyPublicBaseURL)
	sy, err := newTicketSyncer(pg, t, baseURL)
	if err != nil {
		writeErr(w, 400, err.Error())
		return nil
	}
	return sy
}

// pushTickets: POST /api/integrations/trackers/{id}/push {"finding_ids":[...]}
// creates or updates each finding's ticket. Re-pushing is idempotent.
func (s *Server) pushTickets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		FindingIDs []int64 `json:"finding_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || len(req.FindingIDs) == 0 {
		writeErr(w, 400, "finding_ids is required")
		return
	}
	if len(req.FindingIDs) > 200 {
		writeErr(w, 400, "at most 200 findings per push")
		return
	}
	sy := s.enabledSyncer(w, r, pg)
	if sy == nil {
		return
	}
	type item struct {
		FindingID int64             `json:"finding_id"`
		Ticket    *db.FindingTicket `json:"ticket,omitempty"`
		Created   bool              `json:"created"`
		Error     string            `json:"error,omitempty"`
	}
	out := make([]item, 0, len(req.FindingIDs))
	for _, id := range req.FindingIDs {
		it := item{FindingID: id}
		f, err := pg.GetFinding(id)
		if err != nil || f == nil {
			it.Error = "finding not found"
		} else if it.Ticket, it.Created, err = sy.push(r.Context(), f); err != nil {
			it.Error = err.Error()
		}
		out = append(out, it)
	}
	writeJSON(w, 200, map[string]any{"results": out})
}

// syncTickets: POST /api/integrations/trackers/{id}/sync reconciles every
// ticket of the tracker in both directions.
func (s *Server) syncTickets(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	sy := s.enabledSyncer(w, r, pg)
	if sy == nil {
		return
	}
	links, err := pg.FindingTicketsByTracker(r.Context(), sy.tracker.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]ticketSyncResult, 0, len(links))
	for _, l := range links {
		out = append(out, sy.sync(r.Context(), l))
	}
	writeJSON(w, 200, map[string]any{"results": out})
}

// findingIntegrations: GET /api/exploration/findings/{id}/integrations returns
// the finding's tickets and, for an imported finding, its provenance.
func (s *Server) findingIntegrations(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "bad finding id")
		return
	}
	tickets, err := pg.FindingTicketsByFinding(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	src, err := pg.FindingImportSourceByFinding(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if tickets == nil {
		tickets = []*db.FindingTicket{}
	}
	writeJSON(w, 200, map[string]any{"tickets": tickets, "import_source": src})
}
