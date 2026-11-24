-- E10 code-security lane: SAST/SCA imports join the asset and exploration graphs.
-- The lane (runtime | sast | sca) and the code location of each imported finding, so code and
-- runtime findings share one record and one report; backfilled from the import format.
ALTER TABLE finding_import_sources ADD COLUMN IF NOT EXISTS lane     TEXT NOT NULL DEFAULT '';
ALTER TABLE finding_import_sources ADD COLUMN IF NOT EXISTS location TEXT NOT NULL DEFAULT '';
ALTER TABLE finding_import_sources ADD COLUMN IF NOT EXISTS line     INT  NOT NULL DEFAULT 0;
UPDATE finding_import_sources s
   SET lane = CASE WHEN i.format = 'sarif' THEN 'sast' ELSE 'runtime' END
  FROM scan_imports i
 WHERE s.import_id = i.id AND s.lane = '';

-- An import into a task writes one exploration fact (the scan run) that yields its findings, and
-- may anchor its code findings to the runtime assets the scanned code serves.
ALTER TABLE scan_imports ADD COLUMN IF NOT EXISTS fact_node_id     BIGINT REFERENCES exploration_nodes(id) ON DELETE SET NULL;
ALTER TABLE scan_imports ADD COLUMN IF NOT EXISTS anchor_asset_ids JSONB NOT NULL DEFAULT '[]';
