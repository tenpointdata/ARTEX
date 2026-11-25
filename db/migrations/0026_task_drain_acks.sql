-- E7 (c-1d3e0a): worker nodes acknowledge a task's 'deleting' dispatch fence.
-- A row means: this node saw the fence and holds no runs of the task, so it
-- writes nothing more for it (no run starts under the fence). A delete or
-- archive on the control node waits for a row from every live worker node, up
-- to a bound, before removing the task's data. Rows are cleared whenever the
-- fence changes, so an ack never outlives the fence it answers.
CREATE TABLE IF NOT EXISTS task_drain_acks (
    task_id  BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    node_id  TEXT NOT NULL,
    acked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, node_id)
);
