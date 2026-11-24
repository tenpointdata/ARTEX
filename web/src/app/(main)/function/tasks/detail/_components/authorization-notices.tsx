"use client";

import { ShieldAlertIcon } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { t } from "@/i18n";
import type { TaskAuthorizationSummary } from "@/lib/governance-types";

// endedAuthorizationReason is the one sentence every refused resume shows
// (detail header, task list row): why, and what lifts it.
export function endedAuthorizationReason(a: TaskAuthorizationSummary): string {
  return a.revoked
    ? t("Resume is blocked: authorization v{n} was revoked. An admin must store a new version first.", { n: a.version })
    : t("Resume is blocked: authorization v{n} has expired. An admin must store a new version first.", { n: a.version });
}

// AuthorizationEndedNotice explains a task the server paused because its
// engagement authorization ended (cause authorization_ended) and points at the
// one fix, storing a new version, instead of a resume that answers 409.
export function AuthorizationEndedNotice({
  authorization,
  isAdmin,
  onStoreNewVersion,
}: {
  authorization: TaskAuthorizationSummary;
  isAdmin: boolean;
  onStoreNewVersion: () => void;
}) {
  return (
    <Alert variant="destructive" className="py-2">
      <ShieldAlertIcon />
      <AlertTitle>
        {authorization.revoked
          ? t("Paused: the engagement authorization was revoked")
          : t("Paused: the engagement authorization expired")}
      </AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span>
          {t(
            "The target gate refuses every call against a target, so the task cannot resume until a new authorization version is stored.",
          )}
        </span>
        {isAdmin ? (
          <Button size="sm" variant="outline" onClick={onStoreNewVersion}>
            {t("Store a new version")}
          </Button>
        ) : (
          <span className="text-muted-foreground">{t("Ask an admin to store a new version.")}</span>
        )}
      </AlertDescription>
    </Alert>
  );
}
