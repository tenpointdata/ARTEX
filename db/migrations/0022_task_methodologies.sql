-- The idempotent schema supplies the DDL; this migration records the feature version.
-- Operator-chosen methodologies per task (methodology ids); empty = derived from the goal and description.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS methodologies TEXT[] NOT NULL DEFAULT '{}';
