-- RSI Loop C benchmark gate and Loop B lesson graduation (RSI.md, card c-d25ff9).
-- A proposal is one edit inside the search space (rsi.ValidateEdit). Evidence is
-- a held-out baseline/candidate comparison with the server's own verdict. Only an
-- owner action moves a proposal to adopted, and every status change is a row in
-- rsi_adoption_log, which is append-only.

CREATE TABLE IF NOT EXISTS rsi_proposals (
    id            BIGSERIAL PRIMARY KEY,
    round         INT NOT NULL CHECK (round >= 0),
    surface       TEXT NOT NULL CHECK (surface IN ('prompt_body','lesson')),
    target        TEXT NOT NULL DEFAULT '',
    body          TEXT NOT NULL DEFAULT '',
    body_sha      TEXT NOT NULL DEFAULT '',
    lesson_ids    BIGINT[] NOT NULL DEFAULT '{}',
    proposed_from JSONB NOT NULL DEFAULT '[]'::jsonb,
    rationale     TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'proposed'
                  CHECK (status IN ('proposed','passed','failed','adopted','rejected','rolled_back')),
    -- prompt_body adoption: the agent_prompts versions before and after, so a
    -- rollback is a pointer change back to prompt_version_before.
    prompt_version_before INT,
    prompt_version_after  INT,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rsi_proposals_round ON rsi_proposals(round, id);
CREATE INDEX IF NOT EXISTS idx_rsi_proposals_status ON rsi_proposals(status, updated_at DESC);

CREATE TABLE IF NOT EXISTS rsi_proposal_evidence (
    id          BIGSERIAL PRIMARY KEY,
    proposal_id BIGINT NOT NULL REFERENCES rsi_proposals(id) ON DELETE CASCADE,
    passed      BOOLEAN NOT NULL,
    verdict     JSONB NOT NULL,
    baseline    JSONB NOT NULL,
    candidate   JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rsi_evidence_proposal ON rsi_proposal_evidence(proposal_id, id DESC);

CREATE TABLE IF NOT EXISTS rsi_adoption_log (
    id          BIGSERIAL PRIMARY KEY,
    proposal_id BIGINT REFERENCES rsi_proposals(id),
    action      TEXT NOT NULL CHECK (action IN ('propose','evidence','adopt','reject','rollback')),
    actor       TEXT NOT NULL,
    detail      JSONB NOT NULL DEFAULT '{}'::jsonb,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rsi_adoption_log_proposal ON rsi_adoption_log(proposal_id, id);

-- Append-only: the log is the audit trail for adoptions and rollbacks.
CREATE OR REPLACE FUNCTION rsi_adoption_log_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'rsi_adoption_log is append-only';
END $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_rsi_adoption_log_immutable ON rsi_adoption_log;
CREATE TRIGGER trg_rsi_adoption_log_immutable BEFORE UPDATE OR DELETE ON rsi_adoption_log
    FOR EACH ROW EXECUTE FUNCTION rsi_adoption_log_immutable();
