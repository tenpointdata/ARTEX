// The session lives in an HttpOnly cookie that the server sets at login and clears
// at logout. Scripts cannot read it, so this module never sees the token: it asks the
// server whether the browser holds a valid session.

export interface CurrentUser {
  id: string;
  name: string;
  username: string;
  email: string;
  avatar: string;
  role: string;
}

import { PERMISSIONS, type Permission, parsePermissions } from "@/lib/permissions";

// The console has a single account.
const OPERATOR: CurrentUser = { id: "1", name: "ARTEX", username: "ARTEX", email: "", avatar: "", role: "operator" };

// One session lookup per page load. An empty answer (signed out, unreachable)
// is not kept, so the next caller after login asks again.
let permissionsOnce: Promise<Permission[]> | null = null;

async function fetchPermissions(): Promise<Permission[]> {
  if (process.env.NEXT_PUBLIC_MOCK === "1") return [...PERMISSIONS];
  try {
    const r = await fetch("/api/auth/session", { credentials: "same-origin" });
    if (!r.ok) return [];
    const body = (await r.json()) as { permissions?: unknown };
    return parsePermissions(body.permissions);
  } catch {
    return [];
  }
}

export const auth = {
  // True when the browser's session cookie is valid. Mock demo: always signed in.
  async isAuthenticated(): Promise<boolean> {
    if (process.env.NEXT_PUBLIC_MOCK === "1") return true;
    try {
      const r = await fetch("/api/auth/session", { credentials: "same-origin" });
      if (!r.ok) return false;
      const body = (await r.json()) as { authenticated?: unknown };
      return body.authenticated === true;
    } catch {
      return false;
    }
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

  // The signed-in role's permissions (empty when signed out or unreachable).
  permissions(): Promise<Permission[]> {
    permissionsOnce ??= fetchPermissions().then((p) => {
      if (p.length === 0) permissionsOnce = null;
      return p;
    });
    return permissionsOnce;
  },

  getCurrentUser(): CurrentUser {
    return OPERATOR;
  },
};
