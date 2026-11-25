-- RSI gate hardening (card c-0d277f).
--
-- 1. task_rsi_arm: the server's own record of which arm a task ran. Written
--    when the task's agents are built (server/task_llm.go agentsForTask): the
--    sha of the prompt body each RSI role runs and the lessons injected into
--    the planner and worker. Evidence must agree with it, so an uploaded arm
--    file cannot claim a prompt or lesson set the task did not run. A row is
--    marked drifted when a role's prompt pointer moves or the agents are
--    rebuilt with a different arm while the task is still live; a drifted
--    task backs no evidence.
-- 2. rsi_suite_runs: a suite run is registered for a proposal before it
--    runs (suite_run id, suite, targets, seed, replicates). Evidence must
--    match one open registration and consumes it, so a proposal cannot run
--    several suites and upload the best.
-- 3. rsi_adoption_log gains register (2), lesson_retire and lesson_revive
--    (a graduated lesson retired or revived by extraction while its proposal
--    says adopted).

CREATE TABLE IF NOT EXISTS task_rsi_arm (
    task_id           BIGINT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    prompt_sha        JSONB NOT NULL DEFAULT '{}'::jsonb,
    lessons_mode      TEXT NOT NULL DEFAULT 'off',
    lesson_ids        BIGINT[] NOT NULL DEFAULT '{}',
    lessons_block_sha TEXT NOT NULL DEFAULT '',
    drifted           BOOLEAN NOT NULL DEFAULT false,
    drift_reason      TEXT NOT NULL DEFAULT '',
    recorded_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A prompt pointer change on an RSI role while a task is live means the task
-- ran more than one body for that role.
CREATE OR REPLACE FUNCTION task_rsi_arm_prompt_drift() RETURNS trigger AS $$
BEGIN
    UPDATE task_rsi_arm a
       SET drifted = true,
           drift_reason = 'prompt of ' || NEW.key || ' changed while the task was live',
           updated_at = now()
      FROM tasks t
     WHERE t.id = a.task_id
       AND t.status IN ('created','running','paused')
       AND NOT a.drifted;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_task_rsi_arm_prompt_drift ON agents;
CREATE TRIGGER trg_task_rsi_arm_prompt_drift AFTER UPDATE OF current_prompt_id ON agents
    FOR EACH ROW
    WHEN (OLD.current_prompt_id IS DISTINCT FROM NEW.current_prompt_id AND NEW.key IN ('goals','planner','worker'))
    EXECUTE FUNCTION task_rsi_arm_prompt_drift();

CREATE TABLE IF NOT EXISTS rsi_suite_runs (
    id            BIGSERIAL PRIMARY KEY,
    proposal_id   BIGINT NOT NULL REFERENCES rsi_proposals(id) ON DELETE CASCADE,
    suite_run     TEXT NOT NULL UNIQUE CHECK (suite_run <> ''),
    suite         TEXT NOT NULL,
    suite_version TEXT NOT NULL,
    split         TEXT NOT NULL,
    targets       TEXT[] NOT NULL CHECK (cardinality(targets) > 0),
    seed          BIGINT NOT NULL,
    replicates    INT NOT NULL CHECK (replicates > 0),
    evidence_id   BIGINT UNIQUE REFERENCES rsi_proposal_evidence(id),
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rsi_suite_runs_proposal ON rsi_suite_runs(proposal_id, id);

ALTER TABLE rsi_adoption_log DROP CONSTRAINT IF EXISTS rsi_adoption_log_action_check;
ALTER TABLE rsi_adoption_log ADD CONSTRAINT rsi_adoption_log_action_check
    CHECK (action IN ('propose','evidence','adopt','reject','rollback','register','lesson_retire','lesson_revive'));
