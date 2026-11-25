"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";

// Upper bounds of the two message render limits. They mirror the backend, which answers 400 outside [0, max].
export const SUMMARY_LIMIT_MAX = 4000;
export const ASSETS_LIMIT_MAX = 50;

// limitInvalid reports whether a limit input holds anything other than a whole number in [0, max].
// Empty is valid: it is not submitted, so the stored value stays as it is.
export function limitInvalid(raw: string, max: number): boolean {
  if (raw.trim() === "") return false;
  const n = Number(raw);
  return !Number.isInteger(n) || n < 0 || n > max;
}

export interface GlobalSettingsCardProps {
  baseURL: string;
  setBaseURL: (v: string) => void;
  digestMin: string;
  setDigestMin: (v: string) => void;
  summaryMax: string;
  setSummaryMax: (v: string) => void;
  assetsMax: string;
  setAssetsMax: (v: string) => void;
  globalSaving: boolean;
  saveGlobal: () => void;
}

export function GlobalSettingsCard({
  baseURL,
  setBaseURL,
  digestMin,
  setDigestMin,
  summaryMax,
  setSummaryMax,
  assetsMax,
  setAssetsMax,
  globalSaving,
  saveGlobal,
}: GlobalSettingsCardProps) {
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
            onChange={(e) => setAssetsMax(e.target.value)}
            aria-invalid={limitInvalid(assetsMax, ASSETS_LIMIT_MAX)}
          />
          <p className="text-muted-foreground text-xs">
            {t("Most asset names listed for a finding, 0–{max}. 0 = no truncation (all assets listed).", {
              max: ASSETS_LIMIT_MAX,
            })}
          </p>
        </div>
        <div className="sm:col-span-2">
          <Button onClick={saveGlobal} disabled={globalSaving}>
            {t("Save global settings")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
