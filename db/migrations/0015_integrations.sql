-- E2 integrations: findings out to ticket trackers, scanner data in with provenance.

-- A configured external tracker (GitHub Issues, Jira, DefectDojo). kind is validated by the
-- server against the ticketing registry, no CHECK (same as notification_channels.kind).
-- config holds credentials: sealed by db/secrets.go (AAD ticket_trackers.config) and masked
-- on readback with the tracker's SecretKeys.
CREATE TABLE IF NOT EXISTS ticket_trackers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS trg_ticket_trackers_upd ON ticket_trackers;
CREATE TRIGGER trg_ticket_trackers_upd BEFORE UPDATE ON ticket_trackers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- One finding's ticket in one tracker. dedup_key is the stable key the tracker also carries
-- (issue marker / label / unique_id_from_tool), so a lost row still finds its ticket again.
-- synced_status and remote_open/remote_resolution are the last state both sides agreed on:
-- a sync compares each side against them to tell which side changed.
CREATE TABLE IF NOT EXISTS finding_tickets (
    id                BIGSERIAL PRIMARY KEY,
    finding_id        BIGINT NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    tracker_id        BIGINT NOT NULL REFERENCES ticket_trackers(id) ON DELETE CASCADE,
    dedup_key         TEXT NOT NULL,
    external_id       TEXT NOT NULL,
    external_url      TEXT NOT NULL DEFAULT '',
    synced_status     TEXT NOT NULL DEFAULT '',
    remote_open       BOOLEAN NOT NULL DEFAULT true,
    remote_resolution TEXT NOT NULL DEFAULT '',
    last_error        TEXT NOT NULL DEFAULT '',
    synced_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tracker_id, finding_id)
);
CREATE INDEX IF NOT EXISTS idx_finding_tickets_finding ON finding_tickets(finding_id);

-- One uploaded scanner file. file_sha256 + format identify a re-upload.
CREATE TABLE IF NOT EXISTS scan_imports (
    id                 BIGSERIAL PRIMARY KEY,
    format             TEXT NOT NULL,
    tool_name          TEXT NOT NULL DEFAULT '',
    tool_version       TEXT NOT NULL DEFAULT '',
    file_name          TEXT NOT NULL DEFAULT '',
    file_sha256        TEXT NOT NULL,
    task_id            BIGINT REFERENCES tasks(id) ON DELETE SET NULL,
    imported_by        TEXT NOT NULL DEFAULT '',
    records            INT NOT NULL DEFAULT 0,
    assets             INT NOT NULL DEFAULT 0,
    findings_new       INT NOT NULL DEFAULT 0,
    findings_existing  INT NOT NULL DEFAULT 0,
    skipped            INT NOT NULL DEFAULT 0,
    informational      INT NOT NULL DEFAULT 0,
    truncated          BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scan_imports_time ON scan_imports(created_at DESC);

-- Provenance of each asset an import touched (the asset may predate the import).
CREATE TABLE IF NOT EXISTS scan_import_assets (
    import_id  BIGINT NOT NULL REFERENCES scan_imports(id) ON DELETE CASCADE,
    asset_id   BIGINT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    record     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (import_id, asset_id)
);
CREATE INDEX IF NOT EXISTS idx_scan_import_assets_asset ON scan_import_assets(asset_id);

-- Provenance of an imported finding. dedup_key is unique: a re-import of the same result
-- never creates a second finding and never changes the first one's verification.
CREATE TABLE IF NOT EXISTS finding_import_sources (
    finding_id  BIGINT PRIMARY KEY REFERENCES findings(id) ON DELETE CASCADE,
    import_id   BIGINT REFERENCES scan_imports(id) ON DELETE SET NULL,
    tool        TEXT NOT NULL,
    rule_id     TEXT NOT NULL,
    dedup_key   TEXT NOT NULL UNIQUE,
    record      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
