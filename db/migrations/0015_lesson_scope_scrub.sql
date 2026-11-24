-- RSI lessons stop leaking across engagements. scope_key identifies the
-- engagement of a lesson with no company (a digest of the source task's declared
-- scope, or the task itself); a company lesson is scoped by company_id and keeps
-- an empty key. scrubbed marks text written after target-specific strings were
-- replaced with placeholders; only scrubbed lessons may be reused outside their
-- engagement, and only when ARTEX_RSI_LESSONS_SHARED is set. Existing rows keep
-- scope_key='' and scrubbed=false, so an old no-company lesson is no longer
-- selected by any task; re-run POST /api/tasks/{id}/lessons/extract to rebuild
-- it. schema.sql carries the same columns for a fresh install.
ALTER TABLE rsi_lessons ADD COLUMN IF NOT EXISTS scope_key TEXT NOT NULL DEFAULT '';
ALTER TABLE rsi_lessons ADD COLUMN IF NOT EXISTS scrubbed BOOLEAN NOT NULL DEFAULT false;
