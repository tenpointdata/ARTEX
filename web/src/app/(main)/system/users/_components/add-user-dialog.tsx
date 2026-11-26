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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import { type ConsoleUser, ROLES, type Role } from "@/lib/governance-types";

import { roleLabel } from "./role-label";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (u: ConsoleUser) => void;
}

// AddUserDialog creates a console user. The server's 400 (bad username, weak password, bad role) and
// 409 (name taken) text is shown inline so the form can be corrected without losing what was typed.
export function AddUserDialog({ open, onOpenChange, onCreated }: Props) {
  const [username, setUsername] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [role, setRole] = React.useState<Role>("operator");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");

  React.useEffect(() => {
    if (!open) return;
    setUsername("");
    setPassword("");
    setRole("operator");
    setError("");
  }, [open]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const u = await governanceApi.createUser({ username: username.trim(), password, role });
      toast.success(t("User {name} created", { name: u.username }));
      onCreated(u);
      onOpenChange(false);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{t("Add user")}</DialogTitle>
            <DialogDescription>{t("Creates a console login with the chosen role.")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="new-user-name">{t("Username")}</Label>
            <Input
              id="new-user-name"
              autoComplete="off"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="new-user-password">{t("Password")}</Label>
            <Input
              id="new-user-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <div className="grid gap-1.5">
            <Label>{t("Role")}</Label>
            <Select value={role} onValueChange={(v) => setRole(v as Role)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ROLES.map((r) => (
                  <SelectItem key={r} value={r}>
                    {roleLabel(r)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {role === "client_read" && (
              <p className="text-xs text-muted-foreground">
                {t(
                  "A client account sees nothing until it is bound to at least one company; bind it after creating it.",
                )}
              </p>
            )}
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={busy || !username.trim() || !password}>
              {t("Add user")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
