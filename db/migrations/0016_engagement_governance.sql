-- Engagement governance (E5): console users with roles, per-task authorization
-- artifacts, and a tamper-evident, hash-chained audit log.
--
-- Version 0015 is taken by the integrations work (E2) on a parallel branch.
--
-- 1. audit_chain is append-only. Every entry's hash covers the previous
--    entry's hash, so changing, removing or reordering an entry breaks every
--    hash after it. GET /api/intercept/history/chain/verify recomputes it.
-- 2. Row triggers write an entry for every insert, update and delete of
--    intercept_pending (the intercept history), engagement_authorizations and
--    artex_users, in the same transaction as the write. A writer cannot skip
--    the entry, and the entry cannot outlive a rolled-back write.
-- 3. The application appends its own events (logins, refusals by role) with
--    audit_append().

-- ---------------------------------------------------------------------
-- Hash chain
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_chain (
    seq       BIGINT PRIMARY KEY,           -- 1, 2, 3, ... with no gaps
    at        TEXT NOT NULL,                -- UTC, YYYY-MM-DDTHH:MM:SS.ffffffZ, hashed as written
    actor     TEXT NOT NULL,
    kind      TEXT NOT NULL,                -- row.<table> for trigger entries, else an application event
    subject   TEXT NOT NULL DEFAULT '',     -- the row id for trigger entries
    payload   TEXT NOT NULL,                -- JSON, hashed byte for byte as stored
    prev_hash TEXT NOT NULL,
    hash      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_chain_subject ON audit_chain(kind, subject, seq DESC);

-- The newest entry, read under a row lock so appends are serialized.
CREATE TABLE IF NOT EXISTS audit_chain_head (
    id   INT PRIMARY KEY CHECK (id = 1),
    seq  BIGINT NOT NULL,
    hash TEXT NOT NULL
);
INSERT INTO audit_chain_head(id, seq, hash)
VALUES (1, 0, '0000000000000000000000000000000000000000000000000000000000000000')
ON CONFLICT (id) DO NOTHING;

-- audit_field length-prefixes a value so no two field lists hash the same.
-- The Go verifier (db/audit_chain.go, chainHash) computes the same thing.
CREATE OR REPLACE FUNCTION audit_field(v TEXT) RETURNS TEXT
LANGUAGE sql IMMUTABLE AS $$ SELECT octet_length(v)::text || ':' || v || ';' $$;

CREATE OR REPLACE FUNCTION audit_append(p_actor TEXT, p_kind TEXT, p_subject TEXT, p_payload TEXT)
RETURNS BIGINT LANGUAGE plpgsql AS $$
DECLARE
    h_seq  BIGINT;
    h_hash TEXT;
    n_at   TEXT;
    n_hash TEXT;
BEGIN
    SELECT seq, hash INTO h_seq, h_hash FROM audit_chain_head WHERE id = 1 FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'audit_chain_head is missing';
    END IF;
    n_at := to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"');
    n_hash := encode(sha256(convert_to(
        audit_field((h_seq + 1)::text) || audit_field(h_hash) || audit_field(n_at) ||
        audit_field(COALESCE(p_actor, '')) || audit_field(p_kind) || audit_field(COALESCE(p_subject, '')) ||
        audit_field(p_payload), 'UTF8')), 'hex');
    INSERT INTO audit_chain(seq, at, actor, kind, subject, payload, prev_hash, hash)
    VALUES (h_seq + 1, n_at, COALESCE(p_actor, ''), p_kind, COALESCE(p_subject, ''), p_payload, h_hash, n_hash);
    UPDATE audit_chain_head SET seq = h_seq + 1, hash = n_hash WHERE id = 1;
    RETURN h_seq + 1;
END $$;

-- The chain is never edited: an UPDATE, DELETE or TRUNCATE fails. (A database
-- superuser can drop the trigger; the hashes still expose the edit.)
CREATE OR REPLACE FUNCTION audit_chain_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_chain is append-only';
END $$;
DROP TRIGGER IF EXISTS trg_audit_chain_immutable ON audit_chain;
CREATE TRIGGER trg_audit_chain_immutable BEFORE UPDATE OR DELETE ON audit_chain
    FOR EACH ROW EXECUTE FUNCTION audit_chain_immutable();
DROP TRIGGER IF EXISTS trg_audit_chain_no_truncate ON audit_chain;
CREATE TRIGGER trg_audit_chain_no_truncate BEFORE TRUNCATE ON audit_chain
    FOR EACH STATEMENT EXECUTE FUNCTION audit_chain_immutable();

-- audit_row_payload is what an entry records about a row: the row itself, with
-- large or secret columns replaced by their SHA-256. The verifier compares it
-- with the current rows, so a row edited behind the triggers' back shows up.
CREATE OR REPLACE FUNCTION audit_row_payload(tbl TEXT, r JSONB) RETURNS JSONB
LANGUAGE sql IMMUTABLE AS $$
SELECT CASE tbl
    WHEN 'intercept_pending' THEN (r - 'tool_input' - 'audit') || jsonb_build_object(
        'tool_input_sha256', encode(sha256(convert_to(COALESCE(r->'tool_input', 'null'::jsonb)::text, 'UTF8')), 'hex'),
        'audit_sha256', encode(sha256(convert_to(COALESCE(r->'audit', 'null'::jsonb)::text, 'UTF8')), 'hex'),
        'approver', r->'audit'->>'approver')
    WHEN 'artex_users' THEN r - 'password_hash'
    ELSE r
END $$;

-- The actor of a row entry: the transaction's artex.actor setting when the
-- application set one, else the approver or decider the row names.
-- TimeZone is pinned so to_jsonb renders timestamptz columns the same way in
-- every session; the verifier pins it too.
CREATE OR REPLACE FUNCTION audit_row_event() RETURNS trigger
LANGUAGE plpgsql SET TimeZone = 'UTC' AS $$
DECLARE
    rec   JSONB;
    actor TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        rec := to_jsonb(OLD);
    ELSE
        rec := to_jsonb(NEW);
    END IF;
    actor := NULLIF(current_setting('artex.actor', true), '');
    IF actor IS NULL AND TG_TABLE_NAME = 'intercept_pending' THEN
        actor := COALESCE(NULLIF(rec->'audit'->>'approver', ''), NULLIF(rec->>'decided_by', ''));
    END IF;
    PERFORM audit_append(COALESCE(actor, 'system'), 'row.' || TG_TABLE_NAME, rec->>'id',
        jsonb_build_object('op', TG_OP, 'row', audit_row_payload(TG_TABLE_NAME, rec))::text);
    RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS trg_intercept_pending_audit ON intercept_pending;
CREATE TRIGGER trg_intercept_pending_audit AFTER INSERT OR UPDATE OR DELETE ON intercept_pending
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();

-- ---------------------------------------------------------------------
-- Console users and roles
-- ---------------------------------------------------------------------
-- The bootstrap login ARTEX (settings auth.password_hash) is not a row here; it
-- is always admin. version is bumped on every password, role or disabled
-- change, and a session token carries the version it was issued under.
CREATE TABLE IF NOT EXISTS artex_users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE
                    CHECK (username ~ '^[a-z0-9][a-z0-9._-]{1,63}$' AND username NOT IN
                      ('artex','system','anonymous','human','rule','model','target','db','admin')),
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','operator','reviewer','client_read')),
    disabled      BOOLEAN NOT NULL DEFAULT false,
    version       BIGINT NOT NULL DEFAULT 1,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS trg_artex_users_upd ON artex_users;
CREATE TRIGGER trg_artex_users_upd BEFORE UPDATE ON artex_users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
DROP TRIGGER IF EXISTS trg_artex_users_audit ON artex_users;
CREATE TRIGGER trg_artex_users_audit AFTER INSERT OR UPDATE OR DELETE ON artex_users
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();

-- ---------------------------------------------------------------------
-- Engagement authorization artifacts
-- ---------------------------------------------------------------------
-- One task's rules of engagement: what may be touched (scope), when
-- (not_before .. not_after) and how (techniques). Rows are versions and are
-- never updated; a change or a revocation is a new version. The newest version
-- of a task is the one the target gate applies. scope is a JSON array of
-- {"kind": "domain"|"host"|"ip"|"cidr", "value": "..."}; an empty array leaves
-- the targets to the gate's other checks. An empty techniques array allows
-- every technique class.
CREATE TABLE IF NOT EXISTS engagement_authorizations (
    id            BIGSERIAL PRIMARY KEY,
    task_id       BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    version       INT NOT NULL,
    reference     TEXT NOT NULL DEFAULT '',     -- the signed document: SOW, RoE, ticket
    authorized_by TEXT NOT NULL DEFAULT '',     -- who signed it for the client
    scope         JSONB NOT NULL DEFAULT '[]',
    techniques    TEXT[] NOT NULL DEFAULT '{}',
    not_before    TIMESTAMPTZ NOT NULL,
    not_after     TIMESTAMPTZ NOT NULL,
    revoked       BOOLEAN NOT NULL DEFAULT false,
    note          TEXT NOT NULL DEFAULT '',
    created_by    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (not_after > not_before),
    UNIQUE (task_id, version)
);
CREATE OR REPLACE FUNCTION engagement_authorizations_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'engagement_authorizations rows are versions; insert a new version instead';
END $$;
DROP TRIGGER IF EXISTS trg_engagement_authorizations_immutable ON engagement_authorizations;
CREATE TRIGGER trg_engagement_authorizations_immutable BEFORE UPDATE ON engagement_authorizations
    FOR EACH ROW EXECUTE FUNCTION engagement_authorizations_immutable();
DROP TRIGGER IF EXISTS trg_engagement_authorizations_audit ON engagement_authorizations;
CREATE TRIGGER trg_engagement_authorizations_audit AFTER INSERT OR DELETE ON engagement_authorizations
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();
