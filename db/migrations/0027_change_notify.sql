-- The idempotent schema supplies the DDL; this migration records the feature version.
-- =====================================================================
-- Change notifications (the console's live push, GET /api/events/stream)
-- =====================================================================
-- One NOTIFY per changed row on channel artex_changes, delivered at commit, so
-- every writer (agents, imports, restores, other processes) is covered. Payload:
-- {"k": kind, "op": insert|update|delete, "id": row id, "task": task id as text}.
-- TG_ARGV[0] is the kind, TG_ARGV[1] the column holding the task id. Listeners
-- treat the payload as a hint to re-read, never as data.
CREATE OR REPLACE FUNCTION artex_notify_change() RETURNS trigger AS $$
DECLARE
    rec jsonb;
BEGIN
    IF TG_OP = 'DELETE' THEN
        rec := to_jsonb(OLD);
    ELSE
        rec := to_jsonb(NEW);
    END IF;
    PERFORM pg_notify('artex_changes', json_build_object(
        'k', TG_ARGV[0], 'op', lower(TG_OP), 'id', (rec ->> 'id')::bigint,
        'task', COALESCE(rec ->> TG_ARGV[1], ''))::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Findings: every write (triage, severity, report state and retest outcomes all show).
DROP TRIGGER IF EXISTS trg_findings_notify ON findings;
CREATE TRIGGER trg_findings_notify AFTER INSERT OR UPDATE OR DELETE ON findings
    FOR EACH ROW EXECUTE FUNCTION artex_notify_change('finding', 'task_id');

DROP TRIGGER IF EXISTS trg_intercept_pending_notify ON intercept_pending;
CREATE TRIGGER trg_intercept_pending_notify AFTER INSERT OR DELETE ON intercept_pending
    FOR EACH ROW EXECUTE FUNCTION artex_notify_change('approval', 'task_id');
DROP TRIGGER IF EXISTS trg_intercept_pending_notify_upd ON intercept_pending;
CREATE TRIGGER trg_intercept_pending_notify_upd AFTER UPDATE ON intercept_pending
    FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION artex_notify_change('approval', 'task_id');

-- Tasks: only the changes a task list shows (status, pause, queue, soft delete),
-- not the frequent bookkeeping updates.
DROP TRIGGER IF EXISTS trg_tasks_notify ON tasks;
CREATE TRIGGER trg_tasks_notify AFTER INSERT OR DELETE ON tasks
    FOR EACH ROW EXECUTE FUNCTION artex_notify_change('task', 'id');
DROP TRIGGER IF EXISTS trg_tasks_notify_upd ON tasks;
CREATE TRIGGER trg_tasks_notify_upd AFTER UPDATE ON tasks
    FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status
        OR OLD.paused IS DISTINCT FROM NEW.paused
        OR OLD.queued IS DISTINCT FROM NEW.queued
        OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION artex_notify_change('task', 'id');
