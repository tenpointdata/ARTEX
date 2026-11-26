"use client";

import * as React from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { authorizationStatus } from "@/lib/governance";
import { governanceApi } from "@/lib/governance-api";
import type { AuthorizationView } from "@/lib/governance-types";

import { AuthorizationEditor } from "./authorization-form";
import { AuthorizationHistory } from "./authorization-history";
import { StatusBadge, VersionDetails } from "./authorization-parts";

// Authorization tab: the task's engagement authorization artifact (rules of
// engagement the target gate enforces). Everyone with task access reads it;
// only admins can add a version or revoke. See docs/GOVERNANCE.md.
export function AuthorizationTab({ taskId }: { taskId: string }) {
  const isAdmin = useCan("admin");
  const [view, setView] = React.useState<AuthorizationView | null>(null);
  const [error, setError] = React.useState("");
  const [selectedId, setSelectedId] = React.useState<number | null>(null);

  const load = React.useCallback(async () => {
    try {
      setView(await governanceApi.authorization(taskId));
      setError("");
      setSelectedId(null);
    } catch (e) {
      setError(errorText(e));
    }
  }, [taskId]);

  React.useEffect(() => {
    void load();
  }, [load]);

  if (view === null) {
    return error ? (
      <p role="alert" className="text-destructive text-sm">
        {error}
      </p>
    ) : (
      <Skeleton className="h-40 w-full" />
    );
  }

  const versions = view.versions ?? [];
  const current = view.current;
  const status = authorizationStatus(current, view.server_time);
  const selected = versions.find((v) => v.id === selectedId) ?? null;
  // The server's in_force and the client's reading of the window must agree; if
  // they do not (clock skew), the server wins for the badge text of "in force".
  let effective = status;
  if (view.in_force) effective = "in_force";
  else if (status === "in_force") effective = "expired";

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            {t("Engagement authorization")}
            {current ? (
              <span className="font-normal text-muted-foreground text-sm">
                {t("Version {n}", { n: current.version })}
              </span>
            ) : null}
            <StatusBadge status={effective} />
          </CardTitle>
          <CardDescription>
            {t(
              "The rules of engagement the target gate enforces: scope, technique classes and a time window. A task without one is gated as before.",
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          {current ? (
            <VersionDetails a={current} />
          ) : (
            <p className="text-muted-foreground text-sm">{t("This task has no authorization artifact yet.")}</p>
          )}
          {isAdmin ? (
            <AuthorizationEditor
              taskId={taskId}
              current={current}
              techniqueClasses={view.technique_classes ?? []}
              onChanged={load}
            />
          ) : null}
        </CardContent>
      </Card>
      {versions.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("Version history")}</CardTitle>
            <CardDescription>{t("Newest first. Select a version to see its scope and note.")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <AuthorizationHistory
              versions={versions}
              serverTime={view.server_time}
              selectedId={selectedId}
              onSelect={setSelectedId}
            />
            {selected ? (
              <div className="rounded-lg border p-4">
                <p className="mb-3 font-medium text-sm">{t("Version {n}", { n: selected.version })}</p>
                <VersionDetails a={selected} />
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
