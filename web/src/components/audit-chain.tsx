"use client";

import * as React from "react";

import { RefreshCwIcon, ShieldCheckIcon, XIcon } from "lucide-react";

import { AuditEntrySheet } from "@/components/audit-chain-detail";
import { fmtAuditTime, shortHash } from "@/components/audit-chain-format";
import { AuditVerifyResult } from "@/components/audit-chain-verify";
import { CopyButton } from "@/components/copy-button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { AuditEntry, AuditVerification } from "@/lib/governance-types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 100;
const DEBOUNCE_MS = 300;

// Kind identifiers (not translated). `row.` is a prefix filter on the server: it matches every row.* kind.
const KIND_PICKS: { kind: string; label: string }[] = [
  { kind: "row.", label: "row." },
  { kind: "row.intercept_pending", label: "row.intercept_pending" },
  { kind: "row.artex_users", label: "row.artex_users" },
  { kind: "row.engagement_authorizations", label: "row.engagement_authorizations" },
  { kind: "row.artex_user_companies", label: "row.artex_user_companies" },
  { kind: "auth.login", label: "auth.login" },
  { kind: "auth.login_failed", label: "auth.login_failed" },
  { kind: "rbac.denied", label: "rbac.denied" },
  { kind: "rbac.client_scope_denied", label: "rbac.client_scope_denied" },
];

// inButton is true for events that came from a control inside the row (the
// copy button), which must not also open the entry.
function inButton(target: EventTarget): boolean {
  return target instanceof Element && target.closest("button") !== null;
}

// useDebounced returns value once it has been stable for ms.
function useDebounced<T>(value: T, ms: number): T {
  const [out, setOut] = React.useState(value);
  React.useEffect(() => {
    const id = setTimeout(() => setOut(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return out;
}

// AuditChain is the Chain tab of the approval history: the hash-chained audit
// log with filters, forward paging, per-entry detail and a Verify action. It
// needs the read permission, so a role without it makes no calls.
export function AuditChain() {
  const canRead = useCan("read");
  if (!canRead) {
    return (
      <Alert>
        <AlertDescription>{t("Your role does not allow reading the audit chain.")}</AlertDescription>
      </Alert>
    );
  }
  return <AuditChainView />;
}

function AuditChainView() {
  const [kind, setKind] = React.useState("");
  const [subject, setSubject] = React.useState("");
  const appliedKind = useDebounced(kind.trim(), DEBOUNCE_MS);
  const appliedSubject = useDebounced(subject.trim(), DEBOUNCE_MS);

  const [entries, setEntries] = React.useState<AuditEntry[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [hasMore, setHasMore] = React.useState(false);
  const [error, setError] = React.useState("");
  const [reloads, setReloads] = React.useState(0);
  const [selected, setSelected] = React.useState<AuditEntry | null>(null);

  const [verifying, setVerifying] = React.useState(false);
  const [verification, setVerification] = React.useState<AuditVerification | null>(null);
  const [verifyError, setVerifyError] = React.useState("");

  // Guards against a slow response for an older filter overwriting a newer one.
  const generation = React.useRef(0);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reloads is a manual refresh trigger.
  React.useEffect(() => {
    const gen = ++generation.current;
    setLoading(true);
    setError("");
    governanceApi
      .auditChain({ kind: appliedKind, subject: appliedSubject, limit: PAGE_SIZE })
      .then((rows) => {
        if (gen !== generation.current) return;
        setEntries(rows);
        setHasMore(rows.length >= PAGE_SIZE);
      })
      .catch((e: unknown) => {
        if (gen !== generation.current) return;
        setEntries([]);
        setHasMore(false);
        setError(errorText(e));
      })
      .finally(() => {
        if (gen === generation.current) setLoading(false);
      });
  }, [appliedKind, appliedSubject, reloads]);

  async function loadMore() {
    const last = entries[entries.length - 1];
    if (!last || loadingMore) return;
    const gen = generation.current;
    setLoadingMore(true);
    setError("");
    try {
      const rows = await governanceApi.auditChain({
        after_seq: last.seq,
        kind: appliedKind,
        subject: appliedSubject,
        limit: PAGE_SIZE,
      });
      if (gen !== generation.current) return;
      setEntries((prev) => [...prev, ...rows]);
      setHasMore(rows.length >= PAGE_SIZE);
    } catch (e) {
      if (gen === generation.current) setError(errorText(e));
    } finally {
      setLoadingMore(false);
    }
  }

  async function verify() {
    setVerifying(true);
    setVerifyError("");
    try {
      setVerification(await governanceApi.verifyAuditChain());
    } catch (e) {
      setVerification(null);
      setVerifyError(errorText(e));
    } finally {
      setVerifying(false);
    }
  }

  function inspectRow(id: number) {
    setKind("row.intercept_pending");
    setSubject(String(id));
  }

  const brokenAt = verification && !verification.ok ? verification.broken_at : undefined;
  const filtered = kind !== "" || subject !== "";

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <div className="flex flex-wrap items-end gap-2">
        <div className="flex flex-col gap-1 text-muted-foreground text-xs">
          <label htmlFor="audit-chain-kind">{t("Kind")}</label>
          <Input
            id="audit-chain-kind"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            placeholder={t("e.g. rbac.denied, or row. for all row changes")}
            className="w-72"
          />
        </div>
        <div className="flex flex-col gap-1 text-muted-foreground text-xs">
          <label htmlFor="audit-chain-subject">{t("Subject")}</label>
          <Input
            id="audit-chain-subject"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            placeholder={t("Row id or username")}
            className="w-56"
          />
        </div>
        {filtered ? (
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              setKind("");
              setSubject("");
            }}
          >
            <XIcon data-icon="inline-start" />
            {t("Clear filters")}
          </Button>
        ) : null}
        <div className="ml-auto flex gap-2">
          <Button type="button" variant="outline" disabled={loading} onClick={() => setReloads((n) => n + 1)}>
            <RefreshCwIcon data-icon="inline-start" className={cn(loading && "animate-spin")} />
            {t("Refresh")}
          </Button>
          <Button type="button" disabled={verifying} onClick={() => void verify()}>
            <ShieldCheckIcon data-icon="inline-start" />
            {verifying ? t("Verifying…") : t("Verify")}
          </Button>
        </div>
      </div>

      <fieldset
        className="m-0 flex min-w-0 flex-wrap items-center gap-1 border-0 p-0"
        aria-label={t("Quick kind filters")}
      >
        {KIND_PICKS.map((p) => (
          <Button
            key={p.kind}
            type="button"
            size="xs"
            variant={kind === p.kind ? "default" : "outline"}
            title={p.kind === "row." ? t("Prefix filter: every row.* kind") : undefined}
            onClick={() => setKind(kind === p.kind ? "" : p.kind)}
          >
            {p.label}
          </Button>
        ))}
      </fieldset>

      {verifyError ? (
        <Alert variant="destructive">
          <AlertDescription>{verifyError}</AlertDescription>
        </Alert>
      ) : null}
      {verification ? <AuditVerifyResult result={verification} onInspectRow={inspectRow} /> : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="min-w-0 overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">{t("Seq")}</TableHead>
              <TableHead>{t("Time")}</TableHead>
              <TableHead>{t("Actor")}</TableHead>
              <TableHead>{t("Kind")}</TableHead>
              <TableHead>{t("Subject")}</TableHead>
              <TableHead>{t("Hash")}</TableHead>
              <TableHead>{t("Previous hash")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading
              ? Array.from({ length: 5 }, (_, i) => (
                  // biome-ignore lint/suspicious/noArrayIndexKey: static skeleton rows
                  <TableRow key={i}>
                    <TableCell colSpan={7}>
                      <Skeleton className="h-5 w-full" />
                    </TableCell>
                  </TableRow>
                ))
              : null}
            {!loading && entries.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                  {t("No chain entries match.")}
                </TableCell>
              </TableRow>
            ) : null}
            {loading
              ? null
              : entries.map((e) => (
                  <TableRow
                    key={e.seq}
                    tabIndex={0}
                    className={cn("cursor-pointer", brokenAt === e.seq && "bg-destructive/10")}
                    aria-label={t("Chain entry #{seq}", { seq: e.seq })}
                    data-broken={brokenAt === e.seq ? "true" : undefined}
                    onClick={(ev) => {
                      if (!inButton(ev.target)) setSelected(e);
                    }}
                    onKeyDown={(ev) => {
                      if ((ev.key === "Enter" || ev.key === " ") && !inButton(ev.target)) {
                        ev.preventDefault();
                        setSelected(e);
                      }
                    }}
                  >
                    <TableCell className="tabular-nums">{e.seq}</TableCell>
                    <TableCell className="whitespace-nowrap">{fmtAuditTime(e.at)}</TableCell>
                    <TableCell>{e.actor}</TableCell>
                    <TableCell className="font-mono text-xs">{e.kind}</TableCell>
                    <TableCell className="max-w-48 truncate" title={e.subject}>
                      {e.subject || "—"}
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1">
                        <code className="font-mono text-xs" title={e.hash}>
                          {shortHash(e.hash)}
                        </code>
                        <CopyButton text={e.hash} size="icon-xs" variant="ghost" label="" />
                      </span>
                    </TableCell>
                    <TableCell>
                      <code className="font-mono text-muted-foreground text-xs" title={e.prev_hash}>
                        {shortHash(e.prev_hash)}
                      </code>
                    </TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>
      </div>

      {!loading && hasMore ? (
        <Button
          type="button"
          variant="outline"
          className="self-center"
          disabled={loadingMore}
          onClick={() => void loadMore()}
        >
          {loadingMore ? t("Loading…") : t("Load more")}
        </Button>
      ) : null}

      <AuditEntrySheet entry={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
