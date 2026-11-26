import { t } from "@/i18n";
import type { Role } from "@/lib/governance-types";

// roleLabel is the display name of a role. Literal t() calls so the string extractor finds them.
export function roleLabel(r: Role): string {
  switch (r) {
    case "admin":
      return t("Admin");
    case "operator":
      return t("Operator");
    case "reviewer":
      return t("Reviewer");
    case "client_read":
      return t("Client (read-only)");
  }
}
