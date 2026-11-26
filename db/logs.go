package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DBLog is one persisted backend log row from the server_logs table.
type DBLog struct {
	ID        int64
	CreatedAt time.Time
	Level     string
	Tag       string
	Text      string
}

// InsertLog appends one log line and returns its auto-assigned id. The text is
// sealed at rest (secrets_bulk.go explains why the whole line); level and tag
// stay in clear.
func (d *DB) InsertLog(level, tag, text string) (int64, error) {
	sealed, err := sealSecret(colServerLogText, text)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.QueryRowContext(context.Background(),
		"INSERT INTO server_logs(level,tag,text) VALUES($1,$2,$3) RETURNING id",
		level, tag, sealed,
	).Scan(&id)
	return id, err
}

// scanLogs reads log rows newest-first, opens their text and returns them
// oldest-first.
func scanLogs(rows *sql.Rows) ([]*DBLog, error) {
	defer rows.Close()
	var out []*DBLog
	for rows.Next() {
		l := &DBLog{}
		if err := rows.Scan(&l.ID, &l.CreatedAt, &l.Level, &l.Tag, &l.Text); err != nil {
			return nil, err
		}
		text, err := openSecret(colServerLogText, l.Text)
		if err != nil {
			return nil, fmt.Errorf("server_logs id=%d: %w", l.ID, err)
		}
		l.Text = text
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// RecentLogs returns the most recent limit rows, oldest-first.
func (d *DB) RecentLogs(limit int) ([]*DBLog, error) {
	rows, err := d.QueryContext(context.Background(),
		`SELECT id, created_at, level, tag, text
		   FROM server_logs
		  ORDER BY id DESC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return scanLogs(rows)
}

// ListLogsBefore returns up to limit rows with id < beforeID, oldest-first.
func (d *DB) ListLogsBefore(beforeID int64, limit int) ([]*DBLog, error) {
	rows, err := d.QueryContext(context.Background(),
		`SELECT id, created_at, level, tag, text
		   FROM server_logs
		  WHERE id < $1
		  ORDER BY id DESC
		  LIMIT $2`, beforeID, limit)
	if err != nil {
		return nil, err
	}
	return scanLogs(rows)
}
