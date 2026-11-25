// Types for the RSI benchmark gate API (server/rsi_loop.go, db/rsi_loop.go,
// rsi/gate.go). Field names mirror the Go JSON tags.

export type RSIStatus = "proposed" | "passed" | "failed" | "adopted" | "rejected" | "rolled_back";
export type RSISurface = "prompt_body" | "lesson";

export interface RSIProposal {
  id: number;
  round: number;
  surface: RSISurface;
  target?: string;
  body?: string;
  body_sha?: string;
  lesson_ids: number[];
  proposed_from: string[] | null;
  rationale: string;
  status: RSIStatus;
  prompt_version_before?: number;
  prompt_version_after?: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

// Rate is passes over a denominator; never shown without n.
export interface RSIRate {
  passes: number;
  n: number;
}

export interface RSIGateRow {
  gate: string;
  baseline: RSIRate;
  candidate: RSIRate;
  both: number;
  neither: number;
  gained: number;
  lost: number;
}

export interface RSIRegression {
  module: string;
  gate: string;
  gained: number;
  lost: number;
  targets: string[] | null;
}

export interface RSIVerdict {
  passed: boolean;
  module: string;
  reasons: string[] | null;
  gates: RSIGateRow[] | null;
  regressions: RSIRegression[] | null;
  targets_improved: string[] | null;
  targets: string[] | null;
  task_ids: number[] | null;
  // [baseline, candidate] x [attempted, failed]
  runs: [[number, number], [number, number]];
}

export interface RSIEvidence {
  id: number;
  proposal_id: number;
  passed: boolean;
  verdict: RSIVerdict;
  created_at: string;
}

export interface RSILogEntry {
  id: number;
  proposal_id?: number;
  action: string;
  actor: string;
  detail: Record<string, unknown> | null;
  at: string;
}

export interface RSIProposalList {
  proposals: RSIProposal[] | null;
  // keyed by proposal id; absent when the proposal has no evidence yet
  latest_evidence: Record<string, RSIEvidence> | null;
  loop: boolean;
}

export interface RSIProposalDetail {
  proposal: RSIProposal;
  evidence: RSIEvidence[] | null;
  log: RSILogEntry[] | null;
}

export interface RSILoopStatus {
  loop: "on" | "off";
  flag: string;
  lessons_mode: string;
  budget_by_round: number[];
  adoption_requires: string;
  rollback_always_open: boolean;
  immutable_surfaces: string[] | null;
}

export interface RSIProposalResult {
  proposal: RSIProposal;
}
