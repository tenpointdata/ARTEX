import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import type { InterceptAction, InterceptRule } from "@/lib/types";

import type { RuleForm } from "./rule-form";
import { argvLabel, Field, tRich } from "./shared";

export function RuleSheet({
  open,
  setOpen,
  editing,
  form,
  set,
  regexErr,
  regexWarn,
  saving,
  handleSave,
}: {
  open: boolean;
  setOpen: (open: boolean) => void;
  editing: InterceptRule | null;
  form: RuleForm;
  set: (patch: Partial<RuleForm>) => void;
  regexErr: string;
  regexWarn: boolean;
  saving: boolean;
  handleSave: () => void;
}) {
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent side="right" className="flex flex-col gap-0 p-0 sm:max-w-md">
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>{editing ? t("Edit rule") : t("Create rule")}</SheetTitle>
          <SheetDescription className="text-xs">
            {t("Higher priority matches first; the first matching rule wins and the rest are skipped")}
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-5 space-y-5">
          <Field label={t("Name")}>
            <Input
              placeholder={t("Give the rule a name")}
              value={form.name}
              onChange={(e) => set({ name: e.target.value })}
            />
          </Field>

          <Field label={t("Priority (higher numbers match first)")}>
            <Input
              type="number"
              value={form.priority}
              onChange={(e) => set({ priority: parseInt(e.target.value) || 0 })}
            />
          </Field>

          <Separator />

          <Field label={t("Match target")}>
            <Select
              value={form.match_target}
              onValueChange={(v) => set({ match_target: v as RuleForm["match_target"] })}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tool_name">{t("Tool name (tool_name)")}</SelectItem>
                <SelectItem value="tool_input">{t("Input content (tool_input JSON)")}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t("Match type")}>
            <Select
              value={form.match_type}
              onValueChange={(v) => {
                const match_type = v as RuleForm["match_type"];
                // An argv rule always matches the Bash command in tool_input.
                set(match_type === "argv" ? { match_type, match_target: "tool_input" } : { match_type });
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="string">{t("String contains")}</SelectItem>
                <SelectItem value="regex">{t("Regular expression")}</SelectItem>
                <SelectItem value="argv">{t("Argv prefix (Bash only)")}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          <Field label={t("Pattern")}>
            <Input
              placeholder={
                form.match_type === "regex" ? "^Bash$" : form.match_type === "argv" ? '["git","push"]' : "rm -rf"
              }
              value={form.pattern}
              onChange={(e) => set({ pattern: e.target.value })}
              className={regexErr ? "border-destructive focus-visible:ring-destructive" : ""}
            />
            {regexErr && <p className="text-xs text-destructive mt-1">{regexErr}</p>}
            {regexWarn && (
              <p className="text-xs text-amber-600 mt-1">
                {tRich(
                  "Contains Go RE2 extended syntax (e.g. {example}); the browser cannot preview it, and the server validates on submission",
                  {
                    example: <code className="font-mono">(?i)</code>,
                  },
                )}
              </p>
            )}
            {form.match_type === "argv" && (
              <p className="text-xs text-amber-600 mt-1">
                {t("JSON array of 1 to 8 tokens: the program and its subcommands, no flags.")}{" "}
                {tRich("Matches a single simple Bash command whose prefix is exactly {prefix}.", {
                  prefix: <code className="font-mono">{argvLabel(form.pattern)}</code>,
                })}{" "}
                {tRich("Flags are not constrained: {pattern} also covers {command}.", {
                  pattern: <code className="font-mono">[&quot;git&quot;,&quot;push&quot;]</code>,
                  command: <code className="font-mono">git push --force</code>,
                })}
              </p>
            )}
          </Field>

          <Separator />

          <Field label={t("Interception policy")}>
            <Select value={form.action} onValueChange={(v) => set({ action: v as InterceptAction })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="allow">
                  {t("Allow — pass through directly, skipping the remaining rules")}
                </SelectItem>
                <SelectItem value="deny">{t("Block — cut it off and return a denial message to the model")}</SelectItem>
                <SelectItem value="ask">{t("Ask the user — wait for approval")}</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          {form.action !== "allow" && (
            <Field
              label={
                form.action === "deny" ? t("Denial message (returned to the model)") : t("Approval note (optional)")
              }
            >
              <Textarea
                placeholder={form.action === "deny" ? t("Blocked by the security policy") : ""}
                value={form.message}
                onChange={(e) => set({ message: e.target.value })}
                rows={2}
                className="resize-none"
              />
            </Field>
          )}

          {form.action === "ask" && (
            <>
              <Separator />
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium">{t("Enable approval timeout")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("Handle automatically after the timeout instead of waiting")}
                  </p>
                </div>
                <Switch checked={form.timeout_enabled} onCheckedChange={(v) => set({ timeout_enabled: v })} />
              </div>
              {form.timeout_enabled && (
                <div className="flex items-end gap-3">
                  <Field label={t("Timeout (seconds)")}>
                    <Input
                      type="number"
                      min={5}
                      className="w-28"
                      value={form.timeout_seconds}
                      onChange={(e) => {
                        const n = parseInt(e.target.value, 10);
                        if (n > 0) set({ timeout_seconds: n });
                      }}
                    />
                  </Field>
                  <Field label={t("Timeout action")}>
                    <Select
                      value={form.timeout_action}
                      onValueChange={(v) => set({ timeout_action: v as "deny" | "allow" })}
                    >
                      <SelectTrigger className="w-32">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="deny">{t("Auto-deny")}</SelectItem>
                        <SelectItem value="allow">{t("Auto-allow")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                </div>
              )}
            </>
          )}

          <Separator />

          <div className="flex items-center gap-3">
            <Switch id="rule-enabled" checked={form.enabled} onCheckedChange={(v) => set({ enabled: v })} />
            <Label htmlFor="rule-enabled" className="cursor-pointer">
              {t("Enable this rule")}
            </Label>
          </div>
        </div>

        <SheetFooter className="border-t px-6 py-4 flex-row justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("Cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !!regexErr}>
            {saving ? t("Saving…") : t("Save")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
