"use client";

import * as React from "react";

import { can } from "@/lib/governance";
import type { Permission, SessionInfo } from "@/lib/governance-types";

// The signed-in session (username, role, permissions), loaded once by the main
// layout's auth gate. Components use it to hide actions the role lacks; the API
// enforces the same policy regardless (server/rbac_policy.go).
const SessionContext = React.createContext<SessionInfo>({ authenticated: false });

export function SessionProvider({ value, children }: { value: SessionInfo; children: React.ReactNode }) {
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionInfo {
  return React.useContext(SessionContext);
}

// useCan reports whether the session holds permission p.
export function useCan(p: Permission): boolean {
  return can(React.useContext(SessionContext), p);
}
