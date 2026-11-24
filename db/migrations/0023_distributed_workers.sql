-- E7: queue-backed intent workers on separate hosts (docs/DISTRIBUTED.md).
-- The intent claim itself (state/owner/unclaim_token/last_heartbeat_at) already
-- lives on exploration_nodes; these tables carry the control signals that used
-- to exist only in the control process's memory.

-- Live processes sharing this database (control and worker nodes). A node
-- upserts its row every heartbeat; a row older than the node TTL is stale.
CREATE TABLE IF NOT EXISTS worker_nodes (
    node_id      TEXT PRIMARY KEY,
    role         TEXT NOT NULL CHECK (role IN ('all','control','worker')),
    host         TEXT NOT NULL DEFAULT '',
    version      TEXT NOT NULL DEFAULT '',
    workers      INTEGER NOT NULL DEFAULT 0,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Control requests for one running intent, delivered to whichever node holds
-- its claim: kill (planner kill_work), pause / cancel (user work control) and
-- steer (planner steer_work, with message). A node consumes a signal once.
CREATE TABLE IF NOT EXISTS work_signals (
    id          BIGSERIAL PRIMARY KEY,
    task_id     BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    intent_id   BIGINT NOT NULL,
    action      TEXT NOT NULL CHECK (action IN ('kill','pause','cancel','steer')),
    message     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ,
    consumed_by TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_work_signals_open ON work_signals(intent_id) WHERE consumed_at IS NULL;

-- Dispatch fence: when non-empty no node may claim a new intent of the task
-- ('settling' = timeout or budget wrap-up, 'deleting' = delete in progress).
-- Cleared when the task runs again.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS dispatch_closed TEXT NOT NULL DEFAULT '';

-- Capture-proxy scope tokens minted by worker nodes. Only the SHA-256 of a token
-- is stored: the control node's recording proxy resolves a presented token to
-- its task and applies that task's TargetScope, exactly as for a local token.
CREATE TABLE IF NOT EXISTS proxy_scope_tokens (
    token_sha256 TEXT PRIMARY KEY,
    task_id      BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    node_id      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_proxy_scope_tokens_task ON proxy_scope_tokens(task_id);
