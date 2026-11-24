package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The hash-chained audit log (migration 0016). Entries are written by the
// audit_append() SQL function, from row triggers on the intercept history,
// the users and the authorization artifacts, and from AppendAudit. This file
// reads and verifies them; chainHash must stay identical to audit_append.

// GenesisHash is the prev_hash of the first entry.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Audit event kinds the application appends. Row entries use row.<table>.
const (
	AuditLoginOK       = "auth.login"
	AuditLoginFailed   = "auth.login_failed"
	AuditRoleDenied    = "rbac.denied"
	AuditKindRowPrefix = "row."
)

// AuditEntry is one entry of the chain.
type AuditEntry struct {
	Seq      int64           `json:"seq"`
	At       string          `json:"at"`
	Actor    string          `json:"actor"`
	Kind     string          `json:"kind"`
	Subject  string          `json:"subject"`
	Payload  json.RawMessage `json:"payload"`
	PrevHash string          `json:"prev_hash"`
	Hash     string          `json:"hash"`
}

// auditField length-prefixes v (the SQL audit_field).
func auditField(b *strings.Builder, v string) {
	b.WriteString(strconv.Itoa(len(v)))
	b.WriteByte(':')
	b.WriteString(v)
	b.WriteByte(';')
}

// chainHash is the hash of an entry, given the hash of the one before it.
func chainHash(seq int64, prev, at, actor, kind, subject, payload string) string {
	var b strings.Builder
	for _, f := range []string{strconv.FormatInt(seq, 10), prev, at, actor, kind, subject, payload} {
		auditField(&b, f)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// AppendAudit appends an application event. payload is marshalled to JSON;
// nil records {}.
func (d *DB) AppendAudit(actor, kind, subject string, payload any) (int64, error) {
	raw := []byte("{}")
	if payload != nil {
		var err error
		if raw, err = json.Marshal(payload); err != nil {
			return 0, fmt.Errorf("audit payload: %w", err)
		}
	}
	var seq int64
	err := d.QueryRow(`SELECT audit_append($1, $2, $3, $4)`, actor, kind, subject, string(raw)).Scan(&seq)
	return seq, err
}

// AuditFilter narrows ListAudit. Zero values do not filter.
type AuditFilter struct {
	AfterSeq int64  // entries with seq > AfterSeq
	Kind     string // exact kind, or a prefix ending in "." (row.)
	Subject  string
	Limit    int // default 100, at most 1000
}

// ListAudit returns entries in seq order.
func (d *DB) ListAudit(f AuditFilter) ([]AuditEntry, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 1000 {
		f.Limit = 1000
	}
	kindPrefix := ""
	if strings.HasSuffix(f.Kind, ".") {
		kindPrefix, f.Kind = f.Kind, ""
	}
	rows, err := d.Query(`SELECT seq, at, actor, kind, subject, payload, prev_hash, hash FROM audit_chain
		WHERE seq > $1 AND ($2 = '' OR kind = $2) AND ($3 = '' OR starts_with(kind, $3)) AND ($4 = '' OR subject = $4)
		ORDER BY seq LIMIT $5`, f.AfterSeq, f.Kind, kindPrefix, f.Subject, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var payload string
		if err := rows.Scan(&e.Seq, &e.At, &e.Actor, &e.Kind, &e.Subject, &payload, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditVerification is the result of VerifyAudit.
type AuditVerification struct {
	OK       bool   `json:"ok"`
	Entries  int64  `json:"entries"`
	HeadSeq  int64  `json:"head_seq"`
	HeadHash string `json:"head_hash"`
	// BrokenAt is the first entry whose seq, prev_hash or hash is wrong; 0 when none is.
	BrokenAt int64  `json:"broken_at,omitempty"`
	Problem  string `json:"problem,omitempty"`
	// RowsChecked is how many intercept history rows were compared with their newest entry.
	RowsChecked int64 `json:"rows_checked"`
	// RowsMismatched are rows that differ from their newest entry, or that have
	// none although they are newer than the first entry: edited or inserted
	// behind the triggers' back.
	RowsMismatched []int64 `json:"rows_mismatched"`
	// RowsUnchained are rows older than the chain (written before migration 0016).
	RowsUnchained int64 `json:"rows_unchained"`
}

// verifyBatch is how many entries one query of VerifyAudit reads.
const verifyBatch = 5000

// maxMismatchReport caps the row ids a verification lists.
const maxMismatchReport = 200

// VerifyAudit recomputes every hash of the chain, checks that seq has no gap
// and that the head row names the last entry, then compares each intercept
// history row with the newest entry about it.
func (d *DB) VerifyAudit(ctx context.Context) (AuditVerification, error) {
	return verifyAudit(ctx, d.DB)
}

// auditQuerier is what verification reads through: the pool, or a
// transaction (tests verify a tampered chain inside one and roll it back).
type auditQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func verifyAudit(ctx context.Context, d auditQuerier) (AuditVerification, error) {
	v := AuditVerification{OK: true, RowsMismatched: []int64{}}
	prev, want := GenesisHash, int64(1)
	for {
		rows, err := d.QueryContext(ctx, `SELECT seq, at, actor, kind, subject, payload, prev_hash, hash
			FROM audit_chain WHERE seq >= $1 ORDER BY seq LIMIT $2`, want, verifyBatch)
		if err != nil {
			return v, err
		}
		n := 0
		for rows.Next() {
			n++
			var seq int64
			var at, actor, kind, subject, payload, prevHash, hash string
			if err := rows.Scan(&seq, &at, &actor, &kind, &subject, &payload, &prevHash, &hash); err != nil {
				rows.Close()
				return v, err
			}
			if v.BrokenAt != 0 {
				continue
			}
			switch {
			case seq != want:
				v.BrokenAt, v.Problem = want, fmt.Sprintf("entry %d is missing (next is %d)", want, seq)
			case prevHash != prev:
				v.BrokenAt, v.Problem = seq, fmt.Sprintf("entry %d does not link to entry %d", seq, seq-1)
			case chainHash(seq, prevHash, at, actor, kind, subject, payload) != hash:
				v.BrokenAt, v.Problem = seq, fmt.Sprintf("entry %d was changed after it was written", seq)
			}
			v.Entries++
			prev, want = hash, seq+1
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return v, err
		}
		if n < verifyBatch || v.BrokenAt != 0 {
			break
		}
	}
	if err := d.QueryRowContext(ctx, `SELECT seq, hash FROM audit_chain_head WHERE id = 1`).Scan(&v.HeadSeq, &v.HeadHash); err != nil {
		return v, fmt.Errorf("read audit head: %w", err)
	}
	if v.BrokenAt == 0 && (v.HeadSeq != want-1 || v.HeadHash != prev) {
		v.BrokenAt, v.Problem = want, fmt.Sprintf("the head names entry %d but the last entry is %d: entries were removed", v.HeadSeq, want-1)
	}
	if err := verifyInterceptRows(ctx, d, &v); err != nil {
		return v, err
	}
	v.OK = v.BrokenAt == 0 && len(v.RowsMismatched) == 0
	return v, nil
}

// verifyInterceptRows compares every intercept_pending row with the newest
// row entry about it, through the same audit_row_payload the trigger used.
func verifyInterceptRows(ctx context.Context, d auditQuerier, v *AuditVerification) error {
	var genesis sql.NullString
	if err := d.QueryRowContext(ctx, `SELECT at FROM audit_chain WHERE seq = 1`).Scan(&genesis); err != nil && err != sql.ErrNoRows {
		return err
	}
	rows, err := d.QueryContext(ctx, `
WITH last AS (
    SELECT DISTINCT ON (subject) subject, (payload::jsonb)->'row' AS row
    FROM audit_chain WHERE kind = 'row.intercept_pending' AND (payload::jsonb)->>'op' <> 'DELETE'
    ORDER BY subject, seq DESC)
SELECT p.id,
       last.row IS NOT NULL,
       last.row IS NOT DISTINCT FROM audit_row_payload('intercept_pending', to_jsonb(p)),
       $1::text IS NULL OR p.created_at < $1::timestamptz
FROM intercept_pending p LEFT JOIN last ON last.subject = p.id::text
ORDER BY p.id`, genesis)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var chained, same, legacy bool
		if err := rows.Scan(&id, &chained, &same, &legacy); err != nil {
			return err
		}
		v.RowsChecked++
		switch {
		case chained && same:
		case !chained && legacy:
			v.RowsUnchained++
		default:
			if len(v.RowsMismatched) < maxMismatchReport {
				v.RowsMismatched = append(v.RowsMismatched, id)
			}
		}
	}
	return rows.Err()
}
