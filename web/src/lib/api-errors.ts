import { t } from "@/i18n";
import { isForbidden } from "@/lib/governance";

// errorText is the message to show for a failed API call. A 403 means the
// session's role lacks the permission (server/rbac_policy.go): the user stays
// signed in, and the server's reason is shown after a fixed explanation.
// Only a 401 (handled in http()) ends the session.
export function errorText(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err);
  if (isForbidden(err)) return `${t("Your role does not allow this.")} ${msg}`.trim();
  return msg;
}
