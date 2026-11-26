-- 0019: bind console users to the companies (clients) they may see.
--
-- A client_read user sees only what belongs to a company it is bound to: its
-- findings (by the #425 company rule), and the tasks scoped to that company by
-- an operator (task_scope kind 'company', source other than 'agent'). A
-- client_read user with no binding sees nothing. Bindings are ignored for the
-- other roles, which stay global. Enforced in server/client_scope.go.
--
-- Numbered 0019 because the E1 branch holds 0017 and the audit-chain fix 0018.
CREATE TABLE IF NOT EXISTS artex_user_companies (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES artex_users(id) ON DELETE CASCADE,
    company_id  BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, company_id)
);
CREATE INDEX IF NOT EXISTS idx_artex_user_companies_company ON artex_user_companies(company_id);

-- Who may see which client is an authorization decision: chain every change,
-- at commit like the other audited tables (migration 0018).
DROP TRIGGER IF EXISTS trg_artex_user_companies_audit ON artex_user_companies;
CREATE CONSTRAINT TRIGGER trg_artex_user_companies_audit
    AFTER INSERT OR DELETE ON artex_user_companies
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION audit_row_event();
