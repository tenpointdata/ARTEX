-- Take the audit-chain head lock at commit, not at the write.
--
-- Migration 0016 chained every intercept_pending, artex_users and
-- engagement_authorizations write from a plain AFTER ROW trigger. audit_append()
-- locks audit_chain_head FOR UPDATE, so the first audited write of a
-- transaction held the head until that transaction ended. A task archive
-- (DeleteTaskArchiveSource, which deletes intercept rows early in a long
-- transaction) then stalled every gate decision, login and rbac.denied entry
-- until it committed, and a transaction that wrote an intercept row and then
-- waited on another lock could deadlock with one holding that lock and waiting
-- on the head.
--
-- The same triggers are recreated as constraint triggers, DEFERRABLE INITIALLY
-- DEFERRED. Their events are queued with the row image (OLD/NEW) of the write
-- and run during COMMIT, after every statement of the transaction. The head is
-- then the last lock a transaction takes and is held only while its queued
-- entries are appended, so no audited transaction waits on another's work, and
-- no lock is acquired after the head, which removes the lock-order cycle.
--
-- What is unchanged: entries are still written in the writer's transaction (a
-- rolled-back write leaves no entry, a committed one cannot skip it), the
-- payload still records the row as written, and the chain format, hashes and
-- verifier are the same. What changes:
--   * The actor of a row entry is the transaction's artex.actor as it stands at
--     commit (every writer sets it once, first, so this is the same value).
--   * Chain order follows commit order rather than first-write order, which is
--     also the order in which readers can see the rows.
--   * A transaction cannot ALTER or TRUNCATE an audited table after writing to
--     it ("pending trigger events"); nothing in the application does.
--   * SET CONSTRAINTS ... IMMEDIATE would bring back the old behaviour for that
--     transaction; the application never issues it.

DROP TRIGGER IF EXISTS trg_intercept_pending_audit ON intercept_pending;
CREATE CONSTRAINT TRIGGER trg_intercept_pending_audit
    AFTER INSERT OR UPDATE OR DELETE ON intercept_pending
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();

DROP TRIGGER IF EXISTS trg_artex_users_audit ON artex_users;
CREATE CONSTRAINT TRIGGER trg_artex_users_audit
    AFTER INSERT OR UPDATE OR DELETE ON artex_users
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();

DROP TRIGGER IF EXISTS trg_engagement_authorizations_audit ON engagement_authorizations;
CREATE CONSTRAINT TRIGGER trg_engagement_authorizations_audit
    AFTER INSERT OR DELETE ON engagement_authorizations
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();
