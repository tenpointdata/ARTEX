package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Ticket trackers (GitHub Issues, Jira, DefectDojo) and the finding↔ticket links.
// The adapters live in package ticketing; this file only persists their config
// (sealed at rest, colTrackerConfig) and the last state both sides agreed on.

// ErrTicketTrackerNotFound is returned when a tracker id does not exist.
var ErrTicketTrackerNotFound = errors.New("ticket tracker does not exist")

// TicketTracker is one configured tracker. Config is the opened (plaintext)
// JSON; the API layer masks it before it leaves the server.
type TicketTracker struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

const ticketTrackerCols = `id, name, kind, enabled, config, created_at, updated_at`

func scanTicketTracker(sc interface{ Scan(...any) error }) (*TicketTracker, error) {
	var t TicketTracker
	if err := sc.Scan(&t.ID, &t.Name, &t.Kind, &t.Enabled, &t.Config, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	cfg, err := openSecret(colTrackerConfig, string(t.Config))
	if err != nil {
		return nil, err
	}
	t.Config = json.RawMessage(cfg)
	return &t, nil
}

// ListTicketTrackers returns every tracker, by id.
func (d *DB) ListTicketTrackers(ctx context.Context) ([]*TicketTracker, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+ticketTrackerCols+` FROM ticket_trackers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TicketTracker
	for rows.Next() {
		t, err := scanTicketTracker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TicketTrackerByID returns one tracker or ErrTicketTrackerNotFound.
func (d *DB) TicketTrackerByID(ctx context.Context, id int64) (*TicketTracker, error) {
	t, err := scanTicketTracker(d.QueryRowContext(ctx, `SELECT `+ticketTrackerCols+` FROM ticket_trackers WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTicketTrackerNotFound
	}
	return t, err
}

// SaveTicketTracker inserts (ID 0) or updates a tracker; the config is sealed.
// The caller has already merged a masked update (notify.PrepareConfigUpdateKeys).
func (d *DB) SaveTicketTracker(ctx context.Context, t *TicketTracker) (int64, error) {
	if len(t.Config) == 0 {
		t.Config = json.RawMessage(`{}`)
	}
	cfg, err := sealSecret(colTrackerConfig, string(t.Config))
	if err != nil {
		return 0, err
	}
	if t.ID == 0 {
		var id int64
		err := d.QueryRowContext(ctx, `INSERT INTO ticket_trackers(name,kind,enabled,config) VALUES($1,$2,$3,$4) RETURNING id`,
			t.Name, t.Kind, t.Enabled, cfg).Scan(&id)
		return id, err
	}
	res, err := d.ExecContext(ctx, `UPDATE ticket_trackers SET name=$2, kind=$3, enabled=$4, config=$5 WHERE id=$1`,
		t.ID, t.Name, t.Kind, t.Enabled, cfg)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrTicketTrackerNotFound
	}
	return t.ID, nil
}

// DeleteTicketTracker removes a tracker and (cascade) its ticket links. The
// tickets themselves stay in the external tracker.
func (d *DB) DeleteTicketTracker(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM ticket_trackers WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTicketTrackerNotFound
	}
	return nil
}

// FindingTicket links one finding to its ticket in one tracker. SyncedStatus,
// RemoteOpen and RemoteResolution are the state both sides agreed on at the
// last push or sync.
type FindingTicket struct {
	ID               int64     `json:"id"`
	FindingID        int64     `json:"finding_id"`
	TrackerID        int64     `json:"tracker_id"`
	DedupKey         string    `json:"dedup_key"`
	ExternalID       string    `json:"external_id"`
	ExternalURL      string    `json:"external_url"`
	SyncedStatus     string    `json:"synced_status"`
	RemoteOpen       bool      `json:"remote_open"`
	RemoteResolution string    `json:"remote_resolution"`
	LastError        string    `json:"last_error"`
	SyncedAt         time.Time `json:"synced_at"`
	CreatedAt        time.Time `json:"created_at"`
}

const findingTicketCols = `id, finding_id, tracker_id, dedup_key, external_id, external_url, synced_status,
	remote_open, remote_resolution, last_error, synced_at, created_at`

func scanFindingTickets(rows *sql.Rows) ([]*FindingTicket, error) {
	defer rows.Close()
	var out []*FindingTicket
	for rows.Next() {
		var t FindingTicket
		if err := rows.Scan(&t.ID, &t.FindingID, &t.TrackerID, &t.DedupKey, &t.ExternalID, &t.ExternalURL, &t.SyncedStatus,
			&t.RemoteOpen, &t.RemoteResolution, &t.LastError, &t.SyncedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// FindingTicketsByFinding lists a finding's tickets across trackers.
func (d *DB) FindingTicketsByFinding(ctx context.Context, findingID int64) ([]*FindingTicket, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+findingTicketCols+` FROM finding_tickets WHERE finding_id=$1 ORDER BY tracker_id`, findingID)
	if err != nil {
		return nil, err
	}
	return scanFindingTickets(rows)
}

// FindingTicketsByTracker lists every ticket link of one tracker, oldest sync first.
func (d *DB) FindingTicketsByTracker(ctx context.Context, trackerID int64) ([]*FindingTicket, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+findingTicketCols+` FROM finding_tickets WHERE tracker_id=$1 ORDER BY synced_at, id`, trackerID)
	if err != nil {
		return nil, err
	}
	return scanFindingTickets(rows)
}

// TicketSyncCursor walks a tracker's links in passes, oldest sync first. A
// pass starts at PassStart (database time) and visits each link whose
// synced_at is before it exactly once: a link synced during the pass moves
// past PassStart and waits for the next one, and a link that keeps failing
// (its synced_at does not move) is still passed over by After, so it cannot
// starve the others. The zero value starts a new pass.
type TicketSyncCursor struct {
	PassStart time.Time `json:"pass_start"`
	AfterAt   time.Time `json:"after_synced_at"`
	AfterID   int64     `json:"after_id"`
}

// FindingTicketsPage returns up to limit links of the tracker after the
// cursor, and the cursor for the next call. done reports that the pass ended
// with this page (the returned cursor then starts a new pass).
func (d *DB) FindingTicketsPage(ctx context.Context, trackerID int64, cur TicketSyncCursor, limit int) ([]*FindingTicket, TicketSyncCursor, bool, error) {
	if cur.PassStart.IsZero() {
		if err := d.QueryRowContext(ctx, `SELECT now()`).Scan(&cur.PassStart); err != nil {
			return nil, cur, false, err
		}
		cur.AfterAt, cur.AfterID = time.Time{}, 0
	}
	rows, err := d.QueryContext(ctx, `SELECT `+findingTicketCols+` FROM finding_tickets
WHERE tracker_id=$1 AND synced_at < $2 AND (synced_at, id) > ($3, $4)
ORDER BY synced_at, id LIMIT $5`, trackerID, cur.PassStart, cur.AfterAt, cur.AfterID, limit+1)
	if err != nil {
		return nil, cur, false, err
	}
	out, err := scanFindingTickets(rows)
	if err != nil {
		return nil, cur, false, err
	}
	if len(out) <= limit {
		return out, TicketSyncCursor{}, true, nil
	}
	out = out[:limit]
	last := out[len(out)-1]
	return out, TicketSyncCursor{PassStart: cur.PassStart, AfterAt: last.SyncedAt, AfterID: last.ID}, false, nil
}

// FindingTicket returns the link of one finding in one tracker, nil when none.
func (d *DB) FindingTicket(ctx context.Context, trackerID, findingID int64) (*FindingTicket, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+findingTicketCols+` FROM finding_tickets WHERE tracker_id=$1 AND finding_id=$2`, trackerID, findingID)
	if err != nil {
		return nil, err
	}
	out, err := scanFindingTickets(rows)
	if err != nil || len(out) == 0 {
		return nil, err
	}
	return out[0], nil
}

// UpsertFindingTicket records the agreed state after a successful push or sync
// and clears last_error.
func (d *DB) UpsertFindingTicket(ctx context.Context, t *FindingTicket) error {
	return d.QueryRowContext(ctx, `INSERT INTO finding_tickets
	(finding_id, tracker_id, dedup_key, external_id, external_url, synced_status, remote_open, remote_resolution, last_error, synced_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'',now())
ON CONFLICT (tracker_id, finding_id) DO UPDATE SET dedup_key=EXCLUDED.dedup_key, external_id=EXCLUDED.external_id,
	external_url=EXCLUDED.external_url, synced_status=EXCLUDED.synced_status, remote_open=EXCLUDED.remote_open,
	remote_resolution=EXCLUDED.remote_resolution, last_error='', synced_at=now()
RETURNING id, created_at, synced_at`,
		t.FindingID, t.TrackerID, t.DedupKey, t.ExternalID, t.ExternalURL, t.SyncedStatus, t.RemoteOpen, t.RemoteResolution,
	).Scan(&t.ID, &t.CreatedAt, &t.SyncedAt)
}

// SetFindingTicketError records a failed sync without touching the agreed state.
func (d *DB) SetFindingTicketError(ctx context.Context, id int64, msg string) error {
	_, err := d.ExecContext(ctx, `UPDATE finding_tickets SET last_error=$2 WHERE id=$1`, id, msg)
	return err
}

// DeleteFindingTicketsByTracker drops every link of a tracker. Called when the
// tracker's destination changes: the stored external ids belong to the old
// repository/project and must never be read or written in the new one.
func (d *DB) DeleteFindingTicketsByTracker(ctx context.Context, trackerID int64) (int64, error) {
	res, err := d.ExecContext(ctx, `DELETE FROM finding_tickets WHERE tracker_id=$1`, trackerID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
