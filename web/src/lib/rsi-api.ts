// Client for the RSI benchmark gate (server/rsi_loop.go). Reuses lib/api's
// http() so 401 handling, ApiError and mock mode stay identical.

import { http } from "@/lib/api";
import type { RSILoopStatus, RSIProposalDetail, RSIProposalList, RSIProposalResult } from "@/lib/rsi-types";

const post = (body: unknown): RequestInit => ({ method: "POST", body: JSON.stringify(body) });

export const rsiApi = {
  status: () => http<RSILoopStatus>("/rsi/status"),
  proposals: (limit = 100) => http<RSIProposalList>(`/rsi/proposals?limit=${limit}`),
  proposal: (id: number) => http<RSIProposalDetail>(`/rsi/proposals/${id}`),
  // Adoption is an owner action: the server re-checks the owner password.
  adopt: (id: number, evidenceId: number, password: string) =>
    http<RSIProposalResult>(`/rsi/proposals/${id}/adopt`, post({ evidence_id: evidenceId, password })),
  reject: (id: number, reason: string) => http<RSIProposalResult>(`/rsi/proposals/${id}/reject`, post({ reason })),
  rollback: (id: number, reason: string) => http<RSIProposalResult>(`/rsi/proposals/${id}/rollback`, post({ reason })),
};
