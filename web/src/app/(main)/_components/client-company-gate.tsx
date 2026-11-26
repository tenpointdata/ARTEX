"use client";

import * as React from "react";

import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { isForbidden } from "@/lib/governance";
import { governanceApi } from "@/lib/governance-api";
import { useCompanyStore } from "@/stores/company/company-store";

// ClientCompanyGate holds a client_read session's pages until its bound
// companies are loaded and one is active, so no page sends a request the
// server would refuse (no company while bound to several, or another user's
// persisted pick). Other roles pass straight through.
export function ClientCompanyGate({ children }: { children: React.ReactNode }) {
  const isClient = !useCan("read");
  const bound = useCompanyStore((s) => s.bound);
  const companyId = useCompanyStore((s) => s.companyId);
  const setBound = useCompanyStore((s) => s.setBound);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    if (!isClient) return;
    let disposed = false;
    governanceApi
      .myCompanies()
      .then((rows) => {
        if (!disposed) setBound(rows);
      })
      .catch((e: unknown) => {
        if (disposed) return;
        setError(
          isForbidden(e)
            ? t("This account is not bound to any company yet. Ask an administrator to bind it.")
            : errorText(e),
        );
      });
    return () => {
      disposed = true;
    };
  }, [isClient, setBound]);

  if (!isClient) return <>{children}</>;
  if (error) return <p className="m-6 rounded-md border p-4 text-muted-foreground text-sm">{error}</p>;
  if (bound === null) return null;
  if (bound.length === 0 || companyId === null)
    return (
      <p className="m-6 rounded-md border p-4 text-muted-foreground text-sm">
        {t("This account is not bound to any company yet. Ask an administrator to bind it.")}
      </p>
    );
  return <>{children}</>;
}
