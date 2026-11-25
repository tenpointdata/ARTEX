import * as React from "react";

import { Loader2Icon, PlugZapIcon, PlusIcon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { LLMProfile, LLMRetryOverride } from "@/lib/types";

import { fromStore, NONE, OLLAMA_DEFAULT_BASE_URL, type ProfileFormat, rich, toStore } from "./llm-shared";
import { LimitsSections, ModelField } from "./profile-form-sections";
import { ProfileRetryFields, ZERO_OVERRIDE } from "./retry";

export function ProfileSheet({
  profile,
  open,
  onOpenChange,
  onSaved,
}: {
  profile: LLMProfile | null; // null = create
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onSaved: (id: string) => void;
}) {
  const isNew = !profile;
  const [name, setName] = React.useState("");
  const [format, setFormat] = React.useState<ProfileFormat>("anthropic");
  const [model, setModel] = React.useState("");
  const [baseUrl, setBaseUrl] = React.useState("");
  const [proxy, setProxy] = React.useState("");
  const [apiKey, setApiKey] = React.useState("");
  const [keyHint, setKeyHint] = React.useState("");
  const [rps, setRps] = React.useState("0");
  const [rpm, setRpm] = React.useState("0");
  const [cw, setCw] = React.useState("0"); // context window (K tokens); 0 = default 200K
  const [thinkingType, setThinkingType] = React.useState(NONE);
  const [effort, setEffort] = React.useState(NONE);
  const [priority, setPriority] = React.useState("0"); // pool order; higher = tried earlier
  const [poolExclude, setPoolExclude] = React.useState(false);
  const [streaming, setStreaming] = React.useState(true); // true = streaming (default); false = non-streaming
  const [maxTokens, setMaxTokens] = React.useState("0"); // per-reply output cap; 0 = not sent
  const [maxTokensField, setMaxTokensField] = React.useState(NONE); // which field name carries the cap; NONE = max_tokens
  const [sessionHeaderKey, setSessionHeaderKey] = React.useState(""); // custom session header name; empty = not sent
  const [userAgent, setUserAgent] = React.useState(""); // custom User-Agent; empty = keep the default UA
  const [customHeaders, setCustomHeaders] = React.useState("");
  const [clearCustomHeaders, setClearCustomHeaders] = React.useState(false);
  const [retry, setRetry] = React.useState<LLMRetryOverride>(ZERO_OVERRIDE); // retry override for this profile; all 0 = follow the global
  const [testing, setTesting] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [models, setModels] = React.useState<string[]>([]);
  const [loadingModels, setLoadingModels] = React.useState(false);
  const [modelsOpen, setModelsOpen] = React.useState(false);
  const [modelQuery, setModelQuery] = React.useState(""); // filter text for the loaded model list; reset on open/close

  // Every time it opens, the form is re-filled from the passed-in profile (reset to defaults when creating). Closing and reopening
  // the drawer is a clean start, leaving no leftovers from the previous profile.
  React.useEffect(() => {
    if (!open) return;
    setName(profile?.name ?? "");
    setFormat(
      profile?.format === "openai" ||
        profile?.format === "openai-responses" ||
        profile?.format === "ollama" ||
        profile?.format === "bedrock-gateway"
        ? profile.format
        : "anthropic",
    );
    setModel(profile?.model ?? "");
    setBaseUrl(profile?.base_url ?? "");
    setProxy(profile?.proxy ?? "");
    setRps(String(profile?.rate_per_second ?? 0));
    setRpm(String(profile?.rate_per_minute ?? 0));
    setCw(String(profile?.context_window_k ?? 0));
    setThinkingType(fromStore(profile?.thinking_type));
    setEffort(fromStore(profile?.reasoning_effort));
    setPriority(String(profile?.priority ?? 0));
    setPoolExclude(profile?.pool_exclude ?? false);
    setStreaming(profile?.streaming ?? true);
    setMaxTokens(String(profile?.max_tokens ?? 0));
    setMaxTokensField(fromStore(profile?.max_tokens_field));
    setSessionHeaderKey(profile?.session_header_key ?? "");
    setUserAgent(profile?.user_agent ?? "");
    setCustomHeaders("");
    setClearCustomHeaders(false);
    setRetry(profile?.retry ?? ZERO_OVERRIDE);
    setApiKey("");
    setKeyHint(profile?.api_key_hint ?? "");
    setModels([]);
    setModelsOpen(false);
    setModelQuery("");
  }, [open, profile]);

  const profileId = profile ? Number(profile.id) : undefined;

  // Case-insensitive substring filter, so a provider prefix ("deepseek/") or a
  // fragment ("flash") is enough to narrow a 400-entry list.
  const filteredModels = React.useMemo(() => {
    const q = modelQuery.trim().toLowerCase();
    if (!q) return models;
    return models.filter((m) => m.toLowerCase().includes(q));
  }, [models, modelQuery]);

  async function loadModels() {
    if (loadingModels) return;
    setLoadingModels(true);
    setModels([]);
    setModelQuery("");
    try {
      const r = await api.fetchLLMModels(format, baseUrl, apiKey, proxy, profileId);
      if (r.ok && r.models && r.models.length > 0) {
        setModels(r.models);
        setModelsOpen(true);
        toast.success(t("Loaded {count} models", { count: r.models.length }));
      } else {
        toast.error(t("Failed to load models: {error}", { error: r.error ?? t("no models returned") }));
      }
    } catch (e) {
      toast.error(t("Error loading models: {error}", { error: (e as Error).message }));
    } finally {
      setLoadingModels(false);
    }
  }

  async function testConnection() {
    if (testing) return;
    setTesting(true);
    try {
      // Test with the thinking parameters the profile would actually run, so a model that does not support the field fails here,
      // not when a task is already running. Pass the profile id: with the Key input left empty, the stored Key is used.
      const r = await api.testLLM(
        format,
        model,
        baseUrl,
        apiKey,
        proxy,
        toStore(thinkingType),
        toStore(effort),
        profileId,
        streaming,
        sessionHeaderKey.trim(),
        userAgent.trim(),
        customHeaders,
      );
      // Show the reply too: only when the model demonstrably spoke is this the same as working inside a session.
      if (r.ok)
        toast.success(
          t("Connected · {latency}ms · {model}", { latency: r.latency_ms ?? "?", model: r.model ?? model }),
          {
            description: r.reply ? t("Reply: {reply}", { reply: r.reply }) : undefined,
          },
        );
      else toast.error(t("Connection failed: {error}", { error: r.error ?? t("unknown") }));
    } catch (e) {
      toast.error(t("Test error: {error}", { error: (e as Error).message }));
    } finally {
      setTesting(false);
    }
  }

  async function save() {
    if (!name.trim() || !model.trim()) {
      toast.error(t("Please fill in a name and a model"));
      return;
    }
    if (saving) return;
    setSaving(true);
    try {
      const { id } = await api.saveLLMProfile({
        ...(profile ? { id: Number(profile.id) } : {}),
        name: name.trim(),
        format,
        model: model.trim(),
        base_url: baseUrl.trim(),
        proxy: proxy.trim(),
        api_key: apiKey,
        rate_per_second: Number(rps) || 0,
        rate_per_minute: Number(rpm) || 0,
        context_window_k: Number(cw) || 0,
        thinking_type: toStore(thinkingType),
        reasoning_effort: toStore(effort),
        priority: Number(priority) || 0,
        pool_exclude: poolExclude,
        streaming,
        max_tokens: Math.max(0, Number(maxTokens) || 0),
        // The field-name switch only matters for openai (Chat Completions); every other format falls back to the default,
        // and the backend re-applies the same normalization; this just keeps the UI from sending contradictory values.
        max_tokens_field: format === "openai" ? toStore(maxTokensField) : "",
        session_header_key: sessionHeaderKey.trim(),
        user_agent: userAgent.trim(),
        custom_headers: format === "bedrock-gateway" ? customHeaders : "",
        clear_custom_headers: clearCustomHeaders,
        retry,
      });
      if (isNew)
        toast.success(t('Created: {name} (use "Set as active" on its card to enable it)', { name: name.trim() }));
      else
        toast.success(
          profile?.is_default ? t("Saved; the active profile takes effect immediately, no restart needed") : t("Saved"),
        );
      onSaved(String(id));
      onOpenChange(false);
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  let keyPlaceholder = "sk-…";
  let keyLabel = "API Key";
  let basePlaceholder = "https://api.openai.com/v1";
  if (format === "ollama") {
    keyPlaceholder = t("optional — a local Ollama server runs keyless");
    keyLabel = t("API Key (optional for Ollama)");
    basePlaceholder = OLLAMA_DEFAULT_BASE_URL;
  } else if (format === "bedrock-gateway") {
    keyLabel = "Bearer token";
    basePlaceholder = "https://gateway.example.com/bedrock";
    if (keyHint) keyPlaceholder = t("Set ({hint}); leave empty to keep it", { hint: keyHint });
  } else if (keyHint) {
    keyPlaceholder = t("Set ({hint}); leave empty to keep it", { hint: keyHint });
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex flex-col gap-0 p-0 data-[side=right]:min-w-[420px] data-[side=right]:sm:max-w-xl"
      >
        <SheetHeader className="px-4">
          <SheetTitle className="flex items-center gap-2">
            {isNew ? t("New model profile") : t("Edit: {name}", { name: profile?.name ?? "" })}
            {profile?.is_default && (
              <Badge variant="outline" className="border-amber-400/50 text-amber-500">
                {t("Active")}
              </Badge>
            )}
          </SheetTitle>
          <SheetDescription>
            {isNew
              ? t("New profiles are not activated automatically; use 'Set as active' on its card to enable it.")
              : t("Click save after editing; saving the active profile takes effect immediately for all agents.")}
          </SheetDescription>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="p-name">{t("Name")}</Label>
              <Input
                id="p-name"
                placeholder={t("e.g. OpenAI production")}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label>{t("Format")}</Label>
              <Select value={format} onValueChange={(v) => setFormat(v as ProfileFormat)}>
                <SelectTrigger>
                  <SelectValue placeholder={t("Select a format")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="anthropic">Anthropic</SelectItem>
                  <SelectItem value="openai">OpenAI (Chat Completions)</SelectItem>
                  <SelectItem value="openai-responses">OpenAI (Responses API)</SelectItem>
                  <SelectItem value="ollama">{t("Ollama (local, no key needed)")}</SelectItem>
                  <SelectItem value="bedrock-gateway">Bedrock gateway</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <ModelField
            format={format}
            model={model}
            setModel={setModel}
            models={models}
            filteredModels={filteredModels}
            modelsOpen={modelsOpen}
            setModelsOpen={setModelsOpen}
            modelQuery={modelQuery}
            setModelQuery={setModelQuery}
            loadingModels={loadingModels}
            loadModels={loadModels}
          />

          <div className="grid gap-2">
            <Label htmlFor="p-base-url">
              {format === "bedrock-gateway" ? "Gateway base URL (required)" : t("Base URL (optional)")}
            </Label>
            <Input
              id="p-base-url"
              className="font-mono"
              placeholder={basePlaceholder}
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
            />
          </div>

          {format === "bedrock-gateway" && (
            <div className="grid gap-2">
              <Label htmlFor="p-custom-headers">Custom gateway headers (optional)</Label>
              <textarea
                id="p-custom-headers"
                className="min-h-20 w-full rounded-md border bg-background px-3 py-2 font-mono text-sm"
                placeholder="X-Example-Route: demo"
                value={customHeaders}
                onChange={(e) => {
                  setCustomHeaders(e.target.value);
                  setClearCustomHeaders(false);
                }}
                spellCheck={false}
              />
              <p className="text-muted-foreground text-xs">
                One Name: Value per line. Values are encrypted and cannot be viewed after saving. Leave blank to keep
                saved headers.
              </p>
              {profile?.custom_headers_set && (
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={clearCustomHeaders}
                    onChange={(e) => {
                      setClearCustomHeaders(e.target.checked);
                      if (e.target.checked) setCustomHeaders("");
                    }}
                  />
                  Clear saved custom headers
                </label>
              )}
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="p-proxy">{t("Proxy (optional)")}</Label>
            <Input
              id="p-proxy"
              className="font-mono"
              placeholder="socks5://user:pass@127.0.0.1:1080 · http://127.0.0.1:8080"
              value={proxy}
              onChange={(e) => setProxy(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t(
                "Only outbound LLM requests go through this proxy; http/https/socks5 are supported, optionally with credentials (e.g. socks5://user:pass@host:port — URL-encode the password if it contains special characters). Leave empty for no proxy (direct connection).",
              )}
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-session-header">{t("Custom session header (optional)")}</Label>
            <Input
              id="p-session-header"
              className="font-mono"
              placeholder={t("e.g. x-session-id (empty = not sent)")}
              value={sessionHeaderKey}
              onChange={(e) => setSessionHeaderKey(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {rich(
                t(
                  "Once a header name is set, every request carries this HTTP header, with the value filled in automatically as the <b>current session's session id</b> (chat sessions like conv-12, workers like exp3-worker-i87). Meant for gateways that do prompt caching / sticky routing on a session-id header; stable across turns of one session, distinct across sessions. Leave empty to not send it.",
                ),
              )}
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-user-agent">{t("Custom User-Agent (optional)")}</Label>
            <Input
              id="p-user-agent"
              className="font-mono"
              placeholder={t("e.g. my-app/1.0 (empty = default UA)")}
              value={userAgent}
              onChange={(e) => setUserAgent(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {rich(
                t(
                  "Overrides the <b>User-Agent</b> header on every outbound LLM request. Useful for gateways that route or allow-list by User-Agent. Leave empty to keep the default User-Agent unchanged.",
                ),
              )}
            </p>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="p-api-key">{keyLabel}</Label>
            <Input
              id="p-api-key"
              type="password"
              placeholder={keyPlaceholder}
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              // A masked field with no autocomplete hint looks like this site's
              // login password: password managers fill it and Save persists the
              // wrong secret over the stored key. new-password keeps them out.
              autoComplete="new-password"
              spellCheck={false}
              data-1p-ignore="true"
              data-lpignore="true"
            />
          </div>

          <LimitsSections
            format={format}
            rps={rps}
            setRps={setRps}
            rpm={rpm}
            setRpm={setRpm}
            cw={cw}
            setCw={setCw}
            priority={priority}
            setPriority={setPriority}
            poolExclude={poolExclude}
            setPoolExclude={setPoolExclude}
            streaming={streaming}
            setStreaming={setStreaming}
            maxTokens={maxTokens}
            setMaxTokens={setMaxTokens}
            maxTokensField={maxTokensField}
            setMaxTokensField={setMaxTokensField}
            thinkingType={thinkingType}
            setThinkingType={setThinkingType}
            effort={effort}
            setEffort={setEffort}
          />

          <ProfileRetryFields value={retry} onChange={setRetry} />
        </div>

        <div className="flex gap-2 border-t px-4 py-3">
          <Button variant="outline" onClick={testConnection} disabled={testing}>
            {testing ? <Loader2Icon className="animate-spin" /> : <PlugZapIcon />}
            {testing ? t("Testing…") : t("Test connection")}
          </Button>
          <Button onClick={save} disabled={saving} className="flex-1">
            {saving && <Loader2Icon className="animate-spin" />}
            {!saving && (isNew ? <PlusIcon /> : <SaveIcon />)}
            {isNew ? t("Create") : t("Save")}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
