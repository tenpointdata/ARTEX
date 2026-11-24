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

// The console has a single account.
const OPERATOR: CurrentUser = { id: "1", name: "ARTEX", username: "ARTEX", email: "", avatar: "", role: "operator" };

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

  // Ask the server to clear the session cookie and revoke the session token. The
  // server advances its session epoch, so a copy of the token stops working too.
  async logout(): Promise<void> {
    if (process.env.NEXT_PUBLIC_MOCK === "1") return;
    try {
      await fetch("/api/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // Network failure: the caller still navigates to /login, but the server never
      // saw the request, so the cookie and token stay valid until they expire.
    }
  },

  getCurrentUser(): CurrentUser {
    return OPERATOR;
  },
};
