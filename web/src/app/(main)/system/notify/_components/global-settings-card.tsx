"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import type { NotificationMeta } from "@/lib/types";

// Upper bounds of the two message render limits. They mirror the backend, which answers 400 outside [0, max].
const SUMMARY_LIMIT_MAX = 4000;
const ASSETS_LIMIT_MAX = 50;

// limitInvalid reports whether a limit input holds anything other than a whole number in [0, max].
// Empty is valid: it is not submitted, so the stored value stays as it is.
function limitInvalid(raw: string, max: number): boolean {
  if (raw.trim() === "") return false;
  const n = Number(raw);
  return !Number.isInteger(n) || n < 0 || n > max;
}

// GlobalSettingsCard edits the notification-wide settings (link-back URL, digest interval, message
// render limits). They are saved through PUT /api/settings, which is admin-only, so other roles
// see the values without a save button.
export function GlobalSettingsCard({ meta, onSaved }: { meta: NotificationMeta; onSaved: () => void }) {
  const canAdmin = useCan("admin");
  const [globalSaving, setGlobalSaving] = React.useState(false);
  const [baseURL, setBaseURL] = React.useState(meta.public_base_url);
  const [digestMin, setDigestMin] = React.useState(meta.digest_interval_min);
  const [summaryMax, setSummaryMax] = React.useState("");
  const [assetsMax, setAssetsMax] = React.useState("");

  React.useEffect(() => {
    setBaseURL(meta.public_base_url);
    setDigestMin(meta.digest_interval_min);
  }, [meta.public_base_url, meta.digest_interval_min]);

  const loadLimits = React.useCallback(() => {
    // The two render limits are not part of /notify/meta; they are only exposed through /api/settings.
    api
      .settings()
      .then((s) => {
        setSummaryMax(String(s.notify_summary_max_runes ?? ""));
        setAssetsMax(String(s.notify_assets_max_shown ?? ""));
      })
      .catch((e) => toast.error(t("Failed to load message limits: {error}", { error: errorText(e) })));
  }, []);
  React.useEffect(() => {
    loadLimits();
  }, [loadLimits]);

  async function saveGlobal() {
    // Checked before anything is sent: the backend applies the fields of one PUT in order,
    // so a 400 on a limit would leave the URL and digest interval already saved.
    if (limitInvalid(summaryMax, SUMMARY_LIMIT_MAX)) {
      toast.error(t("Summary length must be a whole number between 0 and {max}", { max: SUMMARY_LIMIT_MAX }));
      return;
    }
    if (limitInvalid(assetsMax, ASSETS_LIMIT_MAX)) {
      toast.error(t("Assets listed must be a whole number between 0 and {max}", { max: ASSETS_LIMIT_MAX }));
      return;
    }
    setGlobalSaving(true);
    try {
      const patch: Record<string, unknown> = { notify_public_base_url: baseURL.trim() };
      const n = Number(digestMin);
      if (Number.isFinite(n) && n > 0) patch.notify_digest_interval_min = n;
      if (summaryMax.trim() !== "") patch.notify_summary_max_runes = Number(summaryMax);
      if (assetsMax.trim() !== "") patch.notify_assets_max_shown = Number(assetsMax);
      await api.setSettings(patch);
      toast.success(t("Saved"));
      onSaved();
      loadLimits();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: errorText(e) }));
    } finally {
      setGlobalSaving(false);
    }
  }

  return (
    <Card className="gap-3">
      <CardHeader>
        <CardTitle className="text-base">{t("Global settings")}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="n-base">{t("Link-back URL")}</Label>
          <Input
            id="n-base"
            placeholder="https://artex.example.com"
            value={baseURL}
            readOnly={!canAdmin}
            onChange={(e) => setBaseURL(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t('The address the "View details" button in messages points to. Leave empty to omit the button.')}
          </p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="n-digest">{t("Digest interval (minutes)")}</Label>
          <Input
            id="n-digest"
            type="number"
            min={1}
            max={1440}
            placeholder="30"
            value={digestMin}
            readOnly={!canAdmin}
            onChange={(e) => setDigestMin(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">{t('Only applies to channels in "digest" mode.')}</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="n-summary-max">{t("Summary length (characters)")}</Label>
          <Input
            id="n-summary-max"
            type="number"
            min={0}
            max={SUMMARY_LIMIT_MAX}
            step={1}
            placeholder="120"
            value={summaryMax}
            readOnly={!canAdmin}
            onChange={(e) => setSummaryMax(e.target.value)}
            aria-invalid={limitInvalid(summaryMax, SUMMARY_LIMIT_MAX)}
          />
          <p className="text-muted-foreground text-xs">
            {t("Longest finding summary shown in a message, 0–{max}. 0 = no truncation.", { max: SUMMARY_LIMIT_MAX })}
          </p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="n-assets-max">{t("Assets listed per finding")}</Label>
          <Input
            id="n-assets-max"
            type="number"
            min={0}
            max={ASSETS_LIMIT_MAX}
            step={1}
            placeholder="3"
            value={assetsMax}
            readOnly={!canAdmin}
            onChange={(e) => setAssetsMax(e.target.value)}
            aria-invalid={limitInvalid(assetsMax, ASSETS_LIMIT_MAX)}
          />
          <p className="text-muted-foreground text-xs">
            {t("Most asset names listed for a finding, 0–{max}. 0 = no truncation (all assets listed).", {
              max: ASSETS_LIMIT_MAX,
            })}
          </p>
        </div>
        {canAdmin && (
          <div className="sm:col-span-2">
            <Button onClick={saveGlobal} disabled={globalSaving}>
              {t("Save global settings")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
