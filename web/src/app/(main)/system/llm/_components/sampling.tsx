"use client";

// Optional per-profile sampling pin for reproducible A/B runs (E8 evaluation suite).
// Mirrors the backend db.Sampling: an absent field is not pinned and nothing is sent.
//   seed         sent as the request "seed" field, openai and ollama formats only
//   temperature  used when the calling code sets no temperature of its own

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";
import type { LLMSampling } from "@/lib/types";

/** Text as typed; converted with samplingFromDraft on save. */
export type SamplingDraft = { seed: string; temperature: string };

export const EMPTY_SAMPLING_DRAFT: SamplingDraft = { seed: "", temperature: "" };

const SEED_FORMATS: readonly string[] = ["openai", "ollama"];
const MAX_TEMPERATURE = 2;

export const seedSupported = (format: string): boolean => SEED_FORMATS.includes(format);

export function draftFromSampling(s: LLMSampling | undefined): SamplingDraft {
  return {
    seed: s?.seed == null ? "" : String(s.seed),
    temperature: s?.temperature == null ? "" : String(s.temperature),
  };
}

/** The pin to save, or an error message. A seed on a format that cannot send it is dropped. */
export function samplingFromDraft(format: string, d: SamplingDraft): { value: LLMSampling } | { error: string } {
  const value: LLMSampling = {};
  const seed = d.seed.trim();
  if (seed !== "" && seedSupported(format)) {
    const n = Number(seed);
    if (!Number.isSafeInteger(n)) return { error: t("Sampling seed must be a whole number") };
    value.seed = n;
  }
  const temp = d.temperature.trim();
  if (temp !== "") {
    const n = Number(temp);
    if (!Number.isFinite(n) || n < 0 || n > MAX_TEMPERATURE) {
      return { error: t("Sampling temperature must be between 0 and 2") };
    }
    value.temperature = n;
  }
  return { value };
}

export function ProfileSamplingFields({
  format,
  value,
  onChange,
}: {
  format: string;
  value: SamplingDraft;
  onChange: (v: SamplingDraft) => void;
}) {
  const canSeed = seedSupported(format);
  return (
    <div className="grid gap-2">
      <Label>{t("Sampling pin (optional, for A/B runs)")}</Label>
      <div className="grid grid-cols-2 gap-2">
        <Input
          aria-label={t("Seed")}
          className="font-mono"
          inputMode="numeric"
          disabled={!canSeed}
          placeholder={canSeed ? t("Seed (empty = not pinned)") : t("Seed: openai / ollama only")}
          value={canSeed ? value.seed : ""}
          onChange={(e) => onChange({ ...value, seed: e.target.value })}
        />
        <Input
          aria-label={t("Temperature")}
          className="font-mono"
          inputMode="decimal"
          placeholder={t("Temperature 0–2 (empty = not pinned)")}
          value={value.temperature}
          onChange={(e) => onChange({ ...value, temperature: e.target.value })}
        />
      </div>
      <p className="text-muted-foreground text-xs">
        {t(
          "Pins sampling so replicates of an evaluation suite vary less; the suite stamps both values. The seed is sent only on the openai and ollama formats. The temperature applies when the calling code sets none of its own.",
        )}
      </p>
    </div>
  );
}
