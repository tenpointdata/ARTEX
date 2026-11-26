"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { ConsoleUser } from "@/lib/governance-types";

interface Props {
  user: ConsoleUser | null;
  onClose: () => void;
  onSaved: (u: ConsoleUser) => void;
}

// PasswordDialog sets a console user's password (PATCH password). The user's existing sessions stop
// working on their next request, because the server bumps the user's version.
export function PasswordDialog({ user, onClose, onSaved }: Props) {
  const [password, setPassword] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");

  // Reset the form each time the dialog opens for a (different) user.
  // biome-ignore lint/correctness/useExhaustiveDependencies: user is the trigger, not a value read in the effect
  React.useEffect(() => {
    setPassword("");
    setError("");
  }, [user]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!user) return;
    setBusy(true);
    setError("");
    try {
      const u = await governanceApi.updateUser(user.id, { password });
      toast.success(t("Password updated for {name}", { name: u.username }));
      onSaved(u);
      onClose();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={user !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{t("Set password")}</DialogTitle>
            <DialogDescription>
              {t("New password for {name}. Their signed-in sessions end on the next request.", {
                name: user?.username ?? "",
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="set-user-password">{t("New password")}</Label>
            <Input
              id="set-user-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={busy || !password}>
              {t("Set password")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
