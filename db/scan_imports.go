package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Scanner imports: one scan_imports row per uploaded file, the assets it
// touched (scan_import_assets) and the findings it created
// (finding_import_sources). An imported finding is a scanner claim, never a
// verified one: it is written unverified at POE 0, and a re-import of the same
// result (same dedup key) leaves the existing finding untouched.

// ScanImport is one uploaded scanner file and its outcome counters.
type ScanImport struct {
	ID               int64     `json:"id"`
	Format           string    `json:"format"`
	ToolName         string    `json:"tool_name"`
	ToolVersion      string    `json:"tool_version"`
	FileName         string    `json:"file_name"`
	FileSHA256       string    `json:"file_sha256"`
	TaskID           *int64    `json:"task_id,omitempty"`
	ImportedBy       string    `json:"imported_by"`
	Records          int       `json:"records"`
	Assets           int       `json:"assets"`
	FindingsNew      int       `json:"findings_new"`
	FindingsExisting int       `json:"findings_existing"`
	Skipped          int       `json:"skipped"`
	Informational    int       `json:"informational"`
	Truncated        bool      `json:"truncated"`
	CreatedAt        time.Time `json:"created_at"`
}

func nullTask(id *int64) any {
	if id == nil || *id <= 0 {
		return nil
	}
	return *id
}

// CreateScanImport inserts the import row (counters are filled by FinishScanImport).
func (d *DB) CreateScanImport(ctx context.Context, im *ScanImport) error {
	return d.QueryRowContext(ctx, `INSERT INTO scan_imports(format, tool_name, tool_version, file_name, file_sha256, task_id, imported_by)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at`,
		im.Format, im.ToolName, im.ToolVersion, im.FileName, im.FileSHA256, nullTask(im.TaskID), im.ImportedBy,
	).Scan(&im.ID, &im.CreatedAt)
}

// FinishScanImport stores the outcome counters.
func (d *DB) FinishScanImport(ctx context.Context, im *ScanImport) error {
	_, err := d.ExecContext(ctx, `UPDATE scan_imports SET records=$2, assets=$3, findings_new=$4, findings_existing=$5,
	skipped=$6, informational=$7, truncated=$8 WHERE id=$1`,
		im.ID, im.Records, im.Assets, im.FindingsNew, im.FindingsExisting, im.Skipped, im.Informational, im.Truncated)
	return err
}

// ListScanImports returns one page of imports, newest first, and the total
// number of imports (for the pager). limit is clamped to 1..500 (default 100);
// a negative offset counts as 0.
func (d *DB) ListScanImports(ctx context.Context, limit, offset int) ([]*ScanImport, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM scan_imports`).Scan(&total); err != nil {
		return nil, 0, err
	}
	out, err := d.scanImportPage(ctx, limit, offset)
	return out, total, err
}

func (d *DB) scanImportPage(ctx context.Context, limit, offset int) ([]*ScanImport, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, format, tool_name, tool_version, file_name, file_sha256, task_id, imported_by,
	records, assets, findings_new, findings_existing, skipped, informational, truncated, created_at
FROM scan_imports ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ScanImport
	for rows.Next() {
		var im ScanImport
		var task sql.NullInt64
		if err := rows.Scan(&im.ID, &im.Format, &im.ToolName, &im.ToolVersion, &im.FileName, &im.FileSHA256, &task, &im.ImportedBy,
			&im.Records, &im.Assets, &im.FindingsNew, &im.FindingsExisting, &im.Skipped, &im.Informational, &im.Truncated, &im.CreatedAt); err != nil {
			return nil, err
		}
		if task.Valid {
			im.TaskID = &task.Int64
		}
		out = append(out, &im)
	}
	return out, rows.Err()
}

// AddScanImportAsset records that an import touched an asset. Idempotent.
func (d *DB) AddScanImportAsset(ctx context.Context, importID, assetID int64, record string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO scan_import_assets(import_id, asset_id, record) VALUES ($1,$2,$3)
ON CONFLICT (import_id, asset_id) DO NOTHING`, importID, assetID, record)
	return err
}

// ImportedFinding is one scanner result to store as an unverified finding.
// Summary must be built from typed fields only; scanner free text belongs in
// Evidence, already framed as untrusted data by the caller.
type ImportedFinding struct {
	ImportID  int64
	TaskID    *int64
	VulnClass string
	Name      string
	Severity  string
	Summary   string
	Evidence  string
	Worker    string
	AssetIDs  []int64
	Tool      string
	RuleID    string
	DedupKey  string
	Record    string
	Note      string // verification note, e.g. "imported from nuclei; not verified"
}

// ErrImportedFindingInvalid is returned for a missing dedup key or bad severity.
var ErrImportedFindingInvalid = errors.New("imported finding needs a dedup key and a valid severity")

// AddImportedFinding inserts the finding and its provenance in one transaction.
// When the dedup key is already known it returns that finding's id with
// created=false and changes nothing: an import never edits, re-opens or
// promotes an existing finding.
func (d *DB) AddImportedFinding(ctx context.Context, f ImportedFinding) (id int64, created bool, err error) {
	if f.DedupKey == "" || !ValidSeverity(f.Severity) {
		return 0, false, ErrImportedFindingInvalid
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	// Serialize writers of one key so two concurrent imports cannot both insert.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('finding_import:' || $1))`, f.DedupKey); err != nil {
		return 0, false, err
	}
	err = tx.QueryRowContext(ctx, `SELECT finding_id FROM finding_import_sources WHERE dedup_key=$1`, f.DedupKey).Scan(&id)
	if err == nil {
		return id, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	aids := f.AssetIDs
	if aids == nil {
		aids = []int64{}
	}
	aidsJSON, _ := json.Marshal(aids)
	err = tx.QueryRowContext(ctx, `INSERT INTO findings
	(task_id, vulnclass, name, severity, summary, evidence, worker, asset_ids, status, verification_status, poe_level, verification_note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,0,$11) RETURNING id`,
		nullTask(f.TaskID), f.VulnClass, f.Name, f.Severity, f.Summary, f.Evidence, f.Worker, string(aidsJSON),
		FindingPending, VerificationUnverified, f.Note).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	var imp any
	if f.ImportID > 0 {
		imp = f.ImportID
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO finding_import_sources(finding_id, import_id, tool, rule_id, dedup_key, record)
VALUES ($1,$2,$3,$4,$5,$6)`, id, imp, f.Tool, f.RuleID, f.DedupKey, f.Record); err != nil {
		return 0, false, err
	}
	return id, true, tx.Commit()
}

// FindingImportSource is the provenance of an imported finding.
type FindingImportSource struct {
	FindingID int64     `json:"finding_id"`
	ImportID  *int64    `json:"import_id,omitempty"`
	Tool      string    `json:"tool"`
	RuleID    string    `json:"rule_id"`
	DedupKey  string    `json:"dedup_key"`
	Record    string    `json:"record"`
	CreatedAt time.Time `json:"created_at"`
}

// FindingImportSourceByFinding returns the provenance of a finding, nil when
// the finding was not imported.
func (d *DB) FindingImportSourceByFinding(ctx context.Context, findingID int64) (*FindingImportSource, error) {
	var s FindingImportSource
	var imp sql.NullInt64
	err := d.QueryRowContext(ctx, `SELECT finding_id, import_id, tool, rule_id, dedup_key, record, created_at
FROM finding_import_sources WHERE finding_id=$1`, findingID).Scan(&s.FindingID, &imp, &s.Tool, &s.RuleID, &s.DedupKey, &s.Record, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if imp.Valid {
		s.ImportID = &imp.Int64
	}
	return &s, nil
}
