package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Code-security lane (E10). A scanner import into a task joins the same two
// graphs runtime work writes to: one exploration fact records the scan run
// (tool, format, counts, file hash) and yields a finding node per new finding,
// and code findings are anchored to the runtime assets the scanned code
// serves. The findings rows carry node_id like a worker's finding, so the task
// report, the planner's finding list and the findings exports all read one
// record whichever lane produced it.

// ErrImportGraph is returned when an import cannot be written into the task's
// exploration graph (no task, the task is archived, or the fact is foreign).
var ErrImportGraph = errors.New("scanner import cannot join the task's exploration graph")

// ErrImportAnchor is returned when an anchor asset does not exist.
var ErrImportAnchor = errors.New("scanner import anchor")

// ImportSourceOrigin is the exploration origin of import-written nodes; the
// tool name follows it ("import:semgrep").
const ImportSourceOrigin = "import"

// AddScanImportFact writes the import's exploration fact into the task's
// graph and records it and the anchors (checked by CheckImportAnchors) on the
// import row. payload must hold typed values only: it
// is shown to the planner.
func (d *DB) AddScanImportFact(ctx context.Context, im *ScanImport, payload map[string]any, anchors []int64) (int64, error) {
	if im == nil || im.TaskID == nil || *im.TaskID <= 0 {
		return 0, ErrImportGraph
	}
	if anchors == nil {
		anchors = []int64{}
	}
	var nodeID int64
	// A plain transaction: an import writes no traffic evidence, so it does
	// not take the global evidence lock, only the task row (archive guard).
	err := d.withTx(ctx, func(tx *sql.Tx) error {
		if err := LockTaskEvidenceTx(tx, *im.TaskID); err != nil {
			return fmt.Errorf("%w: %v", ErrImportGraph, err)
		}
		var expID int64
		if err := tx.QueryRowContext(ctx, `SELECT exploration_id FROM tasks WHERE id=$1`, *im.TaskID).Scan(&expID); err != nil {
			return err
		}
		raw, _ := json.Marshal(payload)
		// The fact carries no anchors: coverage counts an asset as tested when a
		// fact anchors it, and a scanner import tests no runtime asset. The
		// finding nodes carry the anchors instead.
		if err := tx.QueryRowContext(ctx, `INSERT INTO exploration_nodes(exploration_id, kind, payload, priority, state, origin)
VALUES ($1,$2,$3,5,'confirmed',$4) RETURNING id`, expID, KindFact, string(raw), ImportSourceOrigin+":"+im.ToolName).Scan(&nodeID); err != nil {
			return err
		}
		aj, _ := json.Marshal(anchors)
		_, err := tx.ExecContext(ctx, `UPDATE scan_imports SET fact_node_id=$2, anchor_asset_ids=$3::jsonb WHERE id=$1`, im.ID, nodeID, string(aj))
		return err
	})
	if err != nil {
		return 0, err
	}
	im.FactNodeID, im.AnchorAssetIDs = &nodeID, anchors
	return nodeID, nil
}

// CheckImportAnchors validates an import's target task and anchor assets
// before anything is written: the task must exist and accept evidence (not
// archived), and every anchor must already belong to the task, either in its
// declared scope (the coverage target set) or carrying the task in task_ids.
// An anchor decides which client companies see the finding and which assets
// a task deletion treats as the task's own, so an arbitrary asset id is
// refused (ErrImportAnchor).
func (d *DB) CheckImportAnchors(ctx context.Context, taskID int64, anchors []int64) error {
	return d.withTx(ctx, func(tx *sql.Tx) error {
		if err := LockTaskEvidenceTx(tx, taskID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: task %d not found", ErrImportGraph, taskID)
			}
			return fmt.Errorf("%w: %v", ErrImportGraph, err)
		}
		if len(anchors) == 0 {
			return nil
		}
		var expID int64
		if err := tx.QueryRowContext(ctx, `SELECT exploration_id FROM tasks WHERE id=$1`, taskID).Scan(&expID); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `WITH `+covTargetCTE+`
SELECT a.id FROM assets a
WHERE a.id = ANY($3::bigint[]) AND (a.id IN (SELECT id FROM target) OR $1 = ANY(a.task_ids))`, taskID, expID, anchors)
		if err != nil {
			return err
		}
		defer rows.Close()
		ok := map[int64]bool{}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ok[id] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
		var bad []int64
		for _, id := range anchors {
			if !ok[id] {
				bad = append(bad, id)
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("%w: assets %v are not in task %d's scope or asset set", ErrImportAnchor, bad, taskID)
		}
		return nil
	})
}

// importedFindingNodeTx writes a new imported finding as a finding node of the
// task's exploration graph, yielded by the import's fact. The node payload has
// the shape RecordFindingTx writes plus the import provenance (source, lane,
// tool, rule, code location), so readers can tell a scanner claim from a
// worker's finding.
func importedFindingNodeTx(ctx context.Context, tx *sql.Tx, f ImportedFinding) (int64, error) {
	if f.TaskID == nil || *f.TaskID <= 0 {
		return 0, ErrImportGraph
	}
	if err := LockTaskEvidenceTx(tx, *f.TaskID); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrImportGraph, err)
	}
	var expID int64
	err := tx.QueryRowContext(ctx, `SELECT t.exploration_id FROM tasks t
JOIN exploration_nodes n ON n.exploration_id=t.exploration_id AND n.id=$2 AND n.kind=$3
WHERE t.id=$1`, *f.TaskID, f.FactNodeID, KindFact).Scan(&expID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: fact %d is not in task %d's graph", ErrImportGraph, f.FactNodeID, *f.TaskID)
	}
	if err != nil {
		return 0, err
	}
	if len(f.AssetIDs) > 0 {
		if err := checkFactAnchorAssets(tx, f.AssetIDs); err != nil {
			return 0, err
		}
	}
	payload := map[string]any{
		"vulnclass": f.VulnClass, "name": f.Name, "severity": f.Severity, "summary": f.Summary,
		"evidence": map[string]any{"by": f.Worker, "poc": f.Evidence},
		"source":   ImportSourceOrigin, "lane": f.Lane, "tool": f.Tool, "rule_id": f.RuleID,
		"import_id": f.ImportID, "verification": VerificationUnverified,
	}
	if f.Location != "" {
		payload["location"] = f.Location
		if f.Line > 0 {
			payload["line"] = f.Line
		}
	}
	raw, _ := json.Marshal(payload)
	var nodeID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO exploration_nodes(exploration_id, kind, payload, priority, state, origin)
VALUES ($1,$2,$3,9,'confirmed',$4) RETURNING id`, expID, KindFinding, string(raw), f.Worker).Scan(&nodeID); err != nil {
		return 0, err
	}
	for _, a := range f.AssetIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exploration_anchors(node_id, asset_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, nodeID, a); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO exploration_edges(exploration_id, src_id, rel, dst_id) VALUES ($1,$2,$3,$4)`,
		expID, f.FactNodeID, RelYields, nodeID); err != nil {
		return 0, err
	}
	return nodeID, nil
}

// ImportSourcesByFindingIDs returns the import provenance of the given
// findings, keyed by finding id; findings that were not imported are absent.
func (d *DB) ImportSourcesByFindingIDs(ctx context.Context, ids []int64) (map[int64]FindingImportSource, error) {
	out := map[int64]FindingImportSource{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := d.QueryContext(ctx, `SELECT finding_id, import_id, tool, rule_id, dedup_key, record, lane, location, line, created_at
FROM finding_import_sources WHERE finding_id = ANY($1::bigint[])`, ids)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var s FindingImportSource
		var imp sql.NullInt64
		if err := rows.Scan(&s.FindingID, &imp, &s.Tool, &s.RuleID, &s.DedupKey, &s.Record, &s.Lane, &s.Location, &s.Line, &s.CreatedAt); err != nil {
			return out, err
		}
		if imp.Valid {
			s.ImportID = &imp.Int64
		}
		out[s.FindingID] = s
	}
	return out, rows.Err()
}

func (d *DB) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
