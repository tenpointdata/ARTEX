"use client";

import { fmtAuditTime } from "@/components/audit-chain-format";
import { CopyButton } from "@/components/copy-button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { t } from "@/i18n";
import type { AuditEntry } from "@/lib/governance-types";

function Field({ label, text }: { label: string; text: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-muted-foreground text-xs">{label}</span>
        <CopyButton text={text} size="xs" label={t("Copy")} />
      </div>
      <pre className="max-h-96 min-w-0 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted/60 p-3 font-mono text-xs leading-5">
        {text || "—"}
      </pre>
    </div>
  );
}

// AuditEntrySheet shows one chain entry in full: payload and complete hashes.
export function AuditEntrySheet({ entry, onClose }: { entry: AuditEntry | null; onClose: () => void }) {
  return (
    <Sheet open={entry !== null} onOpenChange={(open) => (open ? undefined : onClose())}>
      <SheetContent side="right" className="overflow-y-auto p-4 data-[side=right]:sm:max-w-xl">
        {entry ? (
          <>
            <SheetHeader className="p-0">
              <SheetTitle>{t("Chain entry #{seq}", { seq: entry.seq })}</SheetTitle>
              <SheetDescription>
                {fmtAuditTime(entry.at)} · {entry.actor} · {entry.kind} · {entry.subject || "—"}
              </SheetDescription>
            </SheetHeader>
            <Field label={t("Payload")} text={JSON.stringify(entry.payload, null, 2)} />
            <Field label={t("Hash")} text={entry.hash} />
            <Field label={t("Previous hash")} text={entry.prev_hash} />
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
