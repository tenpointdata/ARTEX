// The session lives in an HttpOnly cookie that the server sets at login and clears
// at logout. Scripts cannot read it, so this module never sees the token: it asks the
// server whether the browser holds a valid session.

import type { SessionInfo } from "@/lib/governance-types";

export interface CurrentUser {
  id: string;
  name: string;
  username: string;
  email: string;
  avatar: string;
  role: string;
}

const SIGNED_OUT: SessionInfo = { authenticated: false };

// The mock demo is always signed in as the bootstrap admin.
const MOCK_SESSION: SessionInfo = {
  authenticated: true,
  username: "ARTEX",
  role: "admin",
  permissions: ["client_read", "read", "review", "operate", "admin"],
};

// currentUserOf maps a session to the sidebar's user record.
export function currentUserOf(s: SessionInfo): CurrentUser {
  const username = s.username ?? "";
  return { id: username, name: username, username, email: s.role ?? "", avatar: "", role: s.role ?? "" };
}

export const auth = {
  // The browser's session: whether its cookie is valid and, if so, the username,
  // role and permissions (GET /api/auth/session answers 200 either way).
  async session(): Promise<SessionInfo> {
    if (process.env.NEXT_PUBLIC_MOCK === "1") return MOCK_SESSION;
    try {
      const r = await fetch("/api/auth/session", { credentials: "same-origin" });
      if (!r.ok) return SIGNED_OUT;
      const body = (await r.json()) as Partial<SessionInfo>;
      if (body.authenticated !== true) return SIGNED_OUT;
      return {
        authenticated: true,
        username: typeof body.username === "string" ? body.username : "",
        role: typeof body.role === "string" ? body.role : "",
        permissions: Array.isArray(body.permissions) ? body.permissions.filter((p) => typeof p === "string") : [],
      };
    } catch {
      return SIGNED_OUT;
    }
  },

  // True when the browser's session cookie is valid.
  async isAuthenticated(): Promise<boolean> {
    return (await auth.session()).authenticated;
  },

  // Ask the server to clear the session cookie.
  async logout(): Promise<void> {
    if (process.env.NEXT_PUBLIC_MOCK === "1") return;
    try {
      await fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // The caller still navigates to /login; the server rejects the stale cookie anyway.
    }
  },
};
