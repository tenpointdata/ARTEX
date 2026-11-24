-- Per-import provenance of a finding. finding_import_sources keeps the first import that created
-- the finding; this table records every import that reported it, including re-reports of a
-- known dedup key (which never touch the finding itself).
CREATE TABLE IF NOT EXISTS finding_import_links (
    finding_id  BIGINT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    import_id   BIGINT NOT NULL REFERENCES scan_imports(id) ON DELETE CASCADE,
    record      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (finding_id, import_id)
);
CREATE INDEX IF NOT EXISTS idx_finding_import_links_import ON finding_import_links(import_id);

INSERT INTO finding_import_links(finding_id, import_id, record, created_at)
SELECT finding_id, import_id, record, created_at FROM finding_import_sources WHERE import_id IS NOT NULL
ON CONFLICT DO NOTHING;
