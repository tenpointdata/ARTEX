"use client";

import { Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useCan } from "@/hooks/use-session";

// RowDeleteButton is the trash icon at the end of a table row. Deleting assets
// and companies needs the operate permission, so it renders nothing without it.
export function RowDeleteButton({ label, onClick }: { label: string; onClick: () => void }) {
  if (!useCan("operate")) return null;
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-7 text-muted-foreground hover:text-destructive"
      onClick={onClick}
      aria-label={label}
    >
      <Trash2Icon className="size-3.5" />
    </Button>
  );
}
