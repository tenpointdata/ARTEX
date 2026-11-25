import { Loader2Icon, RefreshCwIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";

import {
  EFFORT_LEVELS,
  MAX_TOKENS_FIELD_HINTS,
  MAX_TOKENS_FIELDS,
  NONE,
  type ProfileFormat,
  THINKING_TYPES,
} from "./llm-shared";

export function ModelField({
  format,
  model,
  setModel,
  models,
  filteredModels,
  modelsOpen,
  setModelsOpen,
  modelQuery,
  setModelQuery,
  loadingModels,
  loadModels,
}: {
  format: ProfileFormat;
  model: string;
  setModel: (v: string) => void;
  models: string[];
  filteredModels: string[];
  modelsOpen: boolean;
  setModelsOpen: (v: boolean) => void;
  modelQuery: string;
  setModelQuery: (v: string) => void;
  loadingModels: boolean;
  loadModels: () => void;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor="p-model">{t("Model")}</Label>
      <div className="flex gap-2">
        <Input
          id="p-model"
          className="font-mono"
          placeholder={format === "ollama" ? "llama3.2" : "claude-opus-4-8"}
          value={model}
          onChange={(e) => setModel(e.target.value)}
        />
        {/* modal: this Popover's content is portaled to <body>, outside the Sheet's scroll lock,
                  so without modal the list renders but cannot scroll. modal lets it hold the topmost scroll lock itself. */}
        <Popover
          open={modelsOpen}
          onOpenChange={(o) => {
            setModelsOpen(o);
            if (!o) setModelQuery("");
          }}
          modal
        >
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="shrink-0"
              disabled={loadingModels || format === "bedrock-gateway"}
              onClick={loadModels}
              title={t("Load available models from the API")}
            >
              {loadingModels ? <Loader2Icon className="animate-spin" /> : <RefreshCwIcon />}
            </Button>
          </PopoverTrigger>
          {models.length > 0 && (
            <PopoverContent className="w-80 gap-0 p-0" align="end">
              <div className="border-b p-1">
                <Input
                  autoFocus
                  className="h-8 border-0 shadow-none focus-visible:ring-0 dark:bg-transparent"
                  placeholder={t("Search {count} models…", { count: models.length })}
                  value={modelQuery}
                  onChange={(e) => setModelQuery(e.target.value)}
                  spellCheck={false}
                  aria-label={t("Search models")}
                />
              </div>
              <div className="max-h-64 overflow-y-auto overscroll-contain p-1">
                {filteredModels.length === 0 ? (
                  <p className="px-2 py-6 text-center text-muted-foreground text-xs">
                    {t("No model matches “{query}”", { query: modelQuery.trim() })}
                  </p>
                ) : (
                  filteredModels.map((m) => (
                    <button
                      key={m}
                      type="button"
                      className="w-full shrink-0 rounded-md px-2 py-1.5 text-left font-mono text-xs hover:bg-accent hover:text-accent-foreground"
                      onClick={() => {
                        setModel(m);
                        setModelsOpen(false);
                      }}
                    >
                      {m}
                    </button>
                  ))
                )}
              </div>
            </PopoverContent>
          )}
        </Popover>
      </div>
    </div>
  );
}

export function LimitsSections({
  format,
  rps,
  setRps,
  rpm,
  setRpm,
  cw,
  setCw,
  priority,
  setPriority,
  poolExclude,
  setPoolExclude,
  streaming,
  setStreaming,
  maxTokens,
  setMaxTokens,
  maxTokensField,
  setMaxTokensField,
  thinkingType,
  setThinkingType,
  effort,
  setEffort,
}: {
  format: ProfileFormat;
  rps: string;
  setRps: (v: string) => void;
  rpm: string;
  setRpm: (v: string) => void;
  cw: string;
  setCw: (v: string) => void;
  priority: string;
  setPriority: (v: string) => void;
  poolExclude: boolean;
  setPoolExclude: (v: boolean) => void;
  streaming: boolean;
  setStreaming: (v: boolean) => void;
  maxTokens: string;
  setMaxTokens: (v: string) => void;
  maxTokensField: string;
  setMaxTokensField: (v: string) => void;
  thinkingType: string;
  setThinkingType: (v: string) => void;
  effort: string;
  setEffort: (v: string) => void;
}) {
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-3">
        <div className="grid gap-2">
          <Label htmlFor="p-rps">{t("Rate limit per second")}</Label>
          <Input id="p-rps" type="number" min={0} value={rps} onChange={(e) => setRps(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="p-rpm">{t("Rate limit per minute")}</Label>
          <Input id="p-rpm" type="number" min={0} value={rpm} onChange={(e) => setRpm(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="p-cw">{t("Context window (K)")}</Label>
          <Input
            id="p-cw"
            type="number"
            min={0}
            max={1000}
            value={cw}
            onChange={(e) => setCw(e.target.value)}
            placeholder="200"
          />
        </div>
      </div>
      <p className="-mt-2 text-muted-foreground text-xs">
        {t(
          "Rate limits of 0 mean unlimited, shared across all agents. The context window is in K (thousand tokens); 0 = the 200K default, capped at 1000 (i.e. 1M). Setting it too high keeps compression from ever triggering.",
        )}
      </p>

      <div className="grid gap-3 rounded-lg border p-3">
        <div className="flex items-center justify-between gap-4">
          <div className="grid gap-0.5">
            <Label htmlFor="p-priority" className="text-sm">
              {t("Pool priority")}
            </Label>
            <p className="text-muted-foreground text-xs">
              {t(
                "Higher numbers are picked first; the active profile is always first regardless of this value. Profiles with equal priority take turns leading, spreading quota naturally.",
              )}
            </p>
          </div>
          <Input
            id="p-priority"
            type="number"
            className="w-24 shrink-0"
            value={priority}
            onChange={(e) => setPriority(e.target.value)}
          />
        </div>
        <div className="flex items-center justify-between gap-4 border-t pt-3">
          <div className="grid gap-0.5">
            <Label className="text-sm">{t("Excluded from pool")}</Label>
            <p className="text-muted-foreground text-xs">
              {t(
                "When enabled, this profile is never used as a failover target (Agents / tasks can still pin it explicitly). Suited to expensive profiles reserved for one specific Agent that should not be burned through by other agents' failures.",
              )}
            </p>
          </div>
          <Switch checked={poolExclude} onCheckedChange={setPoolExclude} aria-label={t("Excluded from pool")} />
        </div>
        <div className="flex items-center justify-between gap-4 border-t pt-3">
          <div className="grid gap-0.5">
            <Label className="text-sm">{t("Streaming output")}</Label>
            <p className="text-muted-foreground text-xs">
              {t(
                "On (default) streams over SSE, with live in-flight progress and real-time token counting. Off uses true non-streaming (stream:false, returning the full response in one shot) — a way around gateways with broken SSE implementations (empty frames / dropped thinking-field frames), at the cost of losing live in-flight progress.",
              )}
            </p>
          </div>
          <Switch checked={streaming} onCheckedChange={setStreaming} aria-label={t("Streaming output")} />
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border p-3">
        <div className="flex items-center justify-between gap-4">
          <div className="grid gap-0.5">
            <Label htmlFor="p-max-tokens" className="text-sm">
              {t("Output cap · max tokens")}
            </Label>
            <p className="text-muted-foreground text-xs">
              {t(
                'How many tokens a single reply may generate at most; sent with every request. 0 (default) = the field is not sent, leaving it to the server default. This is not the same thing as the "context window" above: that is the model\'s total capacity, used only locally to compute the compression threshold. Set it too low and a reasoning model gets truncated mid-thinking, unable to emit even a one-word answer.',
              )}
            </p>
          </div>
          <Input
            id="p-max-tokens"
            type="number"
            min={0}
            className="w-28 shrink-0"
            value={maxTokens}
            onChange={(e) => setMaxTokens(e.target.value)}
            placeholder="0"
          />
        </div>
        <div className="flex items-center justify-between gap-4 border-t pt-3">
          <div className="grid gap-0.5">
            <Label className="text-sm">{t("Cap field name")}</Label>
            <p className="text-muted-foreground text-xs">{MAX_TOKENS_FIELD_HINTS[format]}</p>
          </div>
          <Select
            value={format === "openai" ? maxTokensField : NONE}
            onValueChange={setMaxTokensField}
            disabled={format !== "openai"}
          >
            <SelectTrigger className="w-56 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {MAX_TOKENS_FIELDS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border p-3">
        <div className="flex items-center justify-between gap-4">
          <div className="grid gap-0.5">
            <Label className="text-sm">{t("Thinking switch · thinking.type")}</Label>
            <p className="text-muted-foreground text-xs">
              {t(
                "Controls whether the thinking field is sent. Not sent = the field is omitted (compatible with models like MiniMax that lack support); Disabled = send disabled; Enabled = send enabled. Independent of the effort below.",
              )}
            </p>
          </div>
          <Select value={thinkingType} onValueChange={setThinkingType}>
            <SelectTrigger className="w-32 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {THINKING_TYPES.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center justify-between gap-4 border-t pt-3">
          <div className="grid gap-0.5">
            <Label className="text-sm">{t("Thinking effort · reasoning_effort")}</Label>
            <p className="text-muted-foreground text-xs">
              {t(
                "A separate effort knob (OpenAI reasoning_effort / Anthropic output_config.effort). Some APIs have no thinking field and activate thinking via effort alone, so it can be set on its own without sending the thinking switch.",
              )}
            </p>
          </div>
          <Select value={effort} onValueChange={setEffort}>
            <SelectTrigger className="w-32 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EFFORT_LEVELS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
    </>
  );
}
