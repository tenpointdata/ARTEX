"use client";

import * as React from "react";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { XIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { t } from "@/i18n";
import { importIdParam } from "@/lib/integrations";

// useImportFilter reads the findings page's import_id deep link (Scanner imports →
// Past imports links here). The filter lives in the URL, not in saved list
// preferences, so it never sticks to a later visit. clear() drops it in place.
export function useImportFilter(): { importId: string; clear: () => void } {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const importId = importIdParam(params.get("import_id"));
  const clear = React.useCallback(() => {
    const next = new URLSearchParams(params.toString());
    next.delete("import_id");
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname);
  }, [params, pathname, router]);
  return { importId, clear };
}

// ImportFilterChip shows the active import filter with a link back to the import and a one-click clear.
export function ImportFilterChip({ importId, onClear }: { importId: string; onClear: () => void }) {
  if (!importId) return null;
  return (
    <Badge variant="secondary" className="gap-1 py-1">
      <Link href={`/system/integrations/imports#import-${importId}`} className="hover:underline">
        {t("Import #{id}", { id: importId })}
      </Link>
      <button
        type="button"
        onClick={onClear}
        aria-label={t("Clear the import filter")}
        className="hover:text-foreground ml-1 rounded-sm"
      >
        <XIcon className="size-3" />
      </button>
    </Badge>
  );
}
