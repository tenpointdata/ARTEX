"use client";

import { useEffect, useState } from "react";
import { FileTextIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { ReportBranding } from "@/lib/types";

const fields: { key: Exclude<keyof ReportBranding, "footer">; label: string }[] = [
  { key: "organization", label: "Organization" },
  { key: "client", label: "Client" },
  { key: "title", label: "Report title" },
  { key: "marking", label: "Handling marking" },
  { key: "accent_color", label: "Accent color" },
];

export function ReportBrandingCard() {
  const canAdmin = useCan("admin");
  const [branding, setBranding] = useState<ReportBranding | null>(null);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    let active = true;
    api.reportBranding().then((value) => { if (active) setBranding(value); })
      .catch((error: Error) => {
        if (active) {
          setLoadError(true);
          toast.error(t("Could not load report branding: {error}", { error: error.message }));
        }
      });
    return () => { active = false; };
  }, []);

  async function save() {
    if (!branding) return;
    setSaving(true);
    try {
      setBranding(await api.setReportBranding(branding));
      toast.success(t("Report branding saved"));
    } catch (error) {
      toast.error(t("Save failed: {error}", { error: (error as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base"><FileTextIcon className="size-4" />{t("Report branding")}</CardTitle>
        <CardDescription>{t("Defaults used for branded HTML findings exports. Leave fields blank to use report defaults.")}</CardDescription>
      </CardHeader>
      <CardContent>
        {branding ? (
          <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void save(); }}>
            {fields.map(({ key, label }) => (
              <div className="space-y-2" key={key}>
                <Label htmlFor={`branding-${key}`}>{t(label)}</Label>
                <Input id={`branding-${key}`} value={branding[key]} disabled={!canAdmin || saving}
                  maxLength={key === "accent_color" ? 7 : 200}
                  placeholder={key === "accent_color" ? "#1f4e79" : undefined}
                  pattern={key === "accent_color" ? "#[0-9a-fA-F]{6}" : undefined}
                  onChange={(event) => setBranding({ ...branding, [key]: event.target.value })} />
              </div>
            ))}
            <div className="space-y-2">
              <Label htmlFor="branding-footer">{t("Report footer")}</Label>
              <Textarea id="branding-footer" value={branding.footer} disabled={!canAdmin || saving} maxLength={1000}
                onChange={(event) => setBranding({ ...branding, footer: event.target.value })} />
            </div>
            {canAdmin && <Button type="submit" disabled={saving}>{t("Save")}</Button>}
          </form>
        ) : <p className="text-sm text-muted-foreground">{loadError ? t("Report branding could not be loaded.") : t("Loading...")}</p>}
      </CardContent>
    </Card>
  );
}
