-- E7 follow-up (c-29ff19): worker session transcripts mirrored into the
-- database (transcriptsync), so any node resumes any intent and the control
-- node's human-message path sees a transcript written on a worker node.
-- The local JSONL file stays the write path; these rows are authoritative.

-- One row per session: the current writer (fencing token) and how many lines
-- are stored. Acquire replaces holder; Append only succeeds for the holder and
-- when lines matches the caller's position.
CREATE TABLE IF NOT EXISTS session_transcripts (
    session_id     TEXT PRIMARY KEY,
    exploration_id BIGINT REFERENCES explorations(id) ON DELETE CASCADE,
    holder         TEXT NOT NULL DEFAULT '',
    lines          INTEGER NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_session_transcripts_exploration ON session_transcripts(exploration_id);

-- The JSONL records, one per line, in order (seq from 0).
CREATE TABLE IF NOT EXISTS session_transcript_lines (
    session_id TEXT NOT NULL REFERENCES session_transcripts(session_id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    line       TEXT NOT NULL,
    PRIMARY KEY (session_id, seq)
);
