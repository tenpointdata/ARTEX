package db

import "encoding/json"

// Archives made before c-f4a69d carry only aggregate_stats.token_profiles, so the
// cost view can place them only for all time by task or profile (see
// foldArchivedCost). Their packages still hold the cold llm_usage rows, from
// which the same cost_cells can be computed after the fact. The server reads the
// packages; this file finds the archives and writes the cells.

// legacyArchiveUsage matches an aggregate with per-profile usage, as legacyUsage.
const legacyArchiveUsage = `jsonb_path_exists(archive.aggregate_stats,
  '$.token_profiles[*] ? (@.calls > 0 || @.input_tokens > 0 || @.output_tokens > 0)')`

// CostCellsBackfill is one ready archive that has usage but no cost_cells.
type CostCellsBackfill struct {
	ArchiveID      int64
	TaskID         int64
	ArchivePath    string
	SHA256         string
	CompressedSize int64
}

// ArchivesMissingCostCells lists the ready archives the cost view reports as
// archived_omitted, oldest first.
func (d *DB) ArchivesMissingCostCells() ([]CostCellsBackfill, error) {
	rows, err := d.Query(`SELECT archive.id, archive.task_id, COALESCE(archive.archive_path,''),
       COALESCE(archive.sha256,''), archive.compressed_size
FROM task_archives archive
JOIN tasks task ON task.id=archive.task_id
WHERE task.archived_at IS NOT NULL AND archive.state='ready'
  AND NOT (archive.aggregate_stats ? 'cost_cells') AND ` + legacyArchiveUsage + `
ORDER BY archive.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostCellsBackfill
	for rows.Next() {
		var c CostCellsBackfill
		if err := rows.Scan(&c.ArchiveID, &c.TaskID, &c.ArchivePath, &c.SHA256, &c.CompressedSize); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BackfillArchiveCostCells writes aggregate_stats.cost_cells for one archive from
// the llm_usage rows of its package (the manifest's tables.llm_usage, rows as
// row_to_json wrote them; columns an older schema lacked read as NULL). It
// changes nothing and returns false when the archive is no longer ready,
// already has cells, or the rows yield no cells: an archive whose package lost
// its usage rows keeps its per-profile totals rather than an empty breakdown.
func (d *DB) BackfillArchiveCostCells(archiveID int64, usageRows json.RawMessage) (bool, error) {
	if len(usageRows) == 0 {
		usageRows = json.RawMessage("[]")
	}
	cells := archiveCostCellsOver(`jsonb_populate_recordset(NULL::llm_usage, $2::jsonb)`)
	res, err := d.Exec(`WITH cells AS (SELECT (`+cells+`) AS v)
UPDATE task_archives SET aggregate_stats=aggregate_stats || jsonb_build_object('cost_cells', (SELECT v FROM cells))
WHERE id=$1 AND state='ready' AND NOT (aggregate_stats ? 'cost_cells')
  AND jsonb_array_length((SELECT v FROM cells)) > 0`, archiveID, string(usageRows))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
