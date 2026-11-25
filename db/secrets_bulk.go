package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

// Bulk payloads at rest: the same AES-256-GCM box, key and "enc:v1:" TEXT
// format as the secret columns (secrets.go), for high-volume tables whose
// payloads can carry target data or credentials:
//
//   - llm_records request_body/response_body/raw_request/raw_response: full
//     prompts and responses, including tool output and captured traffic. The
//     metadata columns (ts, model, profile, session, task, worker, tokens,
//     latency, status, error) stay in clear: the records page filters and
//     groups on them and the cost views read llm_usage, not these bodies.
//   - server_logs.text: the log line. The logs API only pages by id (RecentLogs,
//     ListLogsBefore) and never searches the text in SQL, so sealing the whole
//     line is the smallest sound option: it needs no Go-side search and no
//     secret-pattern list that would miss whatever a log call formats next.
//     level and tag stay in clear (code-chosen, not data).
//
// Differences from secretColumns, all for volume:
//   - not in the startup backfill (which loads a column into memory under the
//     schema lock); BackfillBulkSecrets re-encrypts legacy rows in small batches
//     in the background instead. Until then a legacy plaintext row reads as-is
//     (openSecret passes an unsealed value through), so no migration is needed.
//   - the startup key check and the "encrypted data but no key" refusal probe
//     the newest sealed row by primary key instead of scanning or counting.
var (
	colLLMRecReqBody  = secretColumn{"llm_records", "request_body", false, ""}
	colLLMRecRespBody = secretColumn{"llm_records", "response_body", false, ""}
	colLLMRecRawReq   = secretColumn{"llm_records", "raw_request", false, ""}
	colLLMRecRawResp  = secretColumn{"llm_records", "raw_response", false, ""}
	colServerLogText  = secretColumn{"server_logs", "text", false, ""}
	bulkSecretColumns = []secretColumn{colLLMRecReqBody, colLLMRecRespBody, colLLMRecRawReq, colLLMRecRawResp, colServerLogText}
)

// newestSealedQuery selects the newest sealed value of a bulk column, walking
// the primary key backwards: new rows are sealed, so it stops early.
func newestSealedQuery(c secretColumn) string {
	return `SELECT ` + c.col + ` FROM ` + c.table + ` WHERE ` + c.col + ` LIKE 'enc:v1:%' ORDER BY id DESC LIMIT 1`
}

// checkBulkSecretKey fails when a sealed bulk value does not open with box, so
// a wrong key fails at startup, like the secret columns.
func checkBulkSecretKey(ctx context.Context, conn *sql.Conn, box *secretBox) error {
	for _, c := range bulkSecretColumns {
		var v string
		err := conn.QueryRowContext(ctx, newestSealedQuery(c)).Scan(&v)
		if err == nil {
			_, err = box.decrypt(c.aad(), v)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("secret key check: %w", err)
		}
	}
	return nil
}

// bulkSealedExists reports whether any bulk column holds a sealed value.
func bulkSealedExists(ctx context.Context, conn *sql.Conn) (bool, error) {
	for _, c := range bulkSecretColumns {
		var v string
		err := conn.QueryRowContext(ctx, newestSealedQuery(c)).Scan(&v)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
	}
	return false, nil
}

// sealLLMRecordBodies returns the four body columns of r, sealed.
func sealLLMRecordBodies(r *LLMRecord) (req, resp, rawReq, rawResp string, err error) {
	if req, err = sealSecret(colLLMRecReqBody, r.RequestBody); err != nil {
		return
	}
	if resp, err = sealSecret(colLLMRecRespBody, r.ResponseBody); err != nil {
		return
	}
	if rawReq, err = sealSecret(colLLMRecRawReq, r.RawRequest); err != nil {
		return
	}
	rawResp, err = sealSecret(colLLMRecRawResp, r.RawResponse)
	return
}

// openLLMRecordBodies decrypts the four body columns of r in place; legacy
// plaintext passes through.
func openLLMRecordBodies(r *LLMRecord) (err error) {
	if r.RequestBody, err = openSecret(colLLMRecReqBody, r.RequestBody); err != nil {
		return
	}
	if r.ResponseBody, err = openSecret(colLLMRecRespBody, r.ResponseBody); err != nil {
		return
	}
	if r.RawRequest, err = openSecret(colLLMRecRawReq, r.RawRequest); err != nil {
		return
	}
	r.RawResponse, err = openSecret(colLLMRecRawResp, r.RawResponse)
	return
}

// BulkBackfillBatch is the row count BackfillBulkSecrets seals per query.
const BulkBackfillBatch = 200

// BackfillBulkSecrets seals the legacy plaintext values of the bulk columns,
// batch rows at a time with pause between batches, until none are left or ctx
// ends. It is optional (reads accept plaintext) and safe to run concurrently
// on several instances: each UPDATE matches the value it read, so a row is
// sealed once and a concurrent write is never overwritten. It returns the
// number of values it sealed. InitSecrets must have run.
func (d *DB) BackfillBulkSecrets(ctx context.Context, batch int, pause time.Duration) (int, error) {
	box := activeSecretBox.Load()
	if box == nil {
		return 0, errNoSecretKey
	}
	if batch <= 0 {
		batch = BulkBackfillBatch
	}
	total := 0
	for _, c := range bulkSecretColumns {
		var cursor int64
		for {
			n, last, err := d.backfillBulkBatch(ctx, box, c, cursor, batch)
			total += n
			if err != nil {
				return total, fmt.Errorf("backfill %s: %w", c.aad(), err)
			}
			if last == 0 {
				break
			}
			cursor = last
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(pause):
			}
		}
	}
	return total, nil
}

// backfillBulkBatch seals up to batch plaintext values of c with id > after.
// last is the highest id it read, 0 when there were none.
func (d *DB) backfillBulkBatch(ctx context.Context, box *secretBox, c secretColumn, after int64, batch int) (sealed int, last int64, err error) {
	rows, err := d.QueryContext(ctx, `SELECT id, `+c.col+` FROM `+c.table+
		` WHERE id > $1 AND `+c.col+` <> '' AND `+c.col+` NOT LIKE 'enc:v1:%' ORDER BY id LIMIT $2`, after, batch)
	if err != nil {
		return 0, 0, err
	}
	var ids []int64
	var olds []string
	for rows.Next() {
		var id int64
		var v string
		if err = rows.Scan(&id, &v); err != nil {
			rows.Close()
			return 0, 0, err
		}
		ids, olds = append(ids, id), append(olds, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, 0, err
	}
	for i, id := range ids {
		nv, err := box.seal(c, olds[i])
		if err != nil {
			return sealed, 0, err
		}
		res, err := d.ExecContext(ctx, `UPDATE `+c.table+` SET `+c.col+`=$1 WHERE id=$2 AND `+c.col+`=$3`, nv, id, olds[i])
		if err != nil {
			return sealed, 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			sealed++
		}
		last = id
	}
	return sealed, last, nil
}

// StartBulkSecretsBackfill runs BackfillBulkSecrets in the background with a
// pause that keeps it a trickle beside live traffic, logging the outcome.
func (d *DB) StartBulkSecretsBackfill(ctx context.Context) {
	go func() {
		n, err := d.BackfillBulkSecrets(ctx, BulkBackfillBatch, 50*time.Millisecond)
		switch {
		case err != nil && ctx.Err() == nil && !errors.Is(err, sql.ErrConnDone) && d.PingContext(ctx) == nil:
			log.Printf("[secrets] bulk backfill stopped after %d value(s): %v", n, err)
		case n > 0:
			log.Printf("[secrets] bulk backfill encrypted %d legacy log/LLM-record value(s)", n)
		}
	}()
}
