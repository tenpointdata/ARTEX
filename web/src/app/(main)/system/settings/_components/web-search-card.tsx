"use client";

import { SearchIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";
import type { Settings } from "@/lib/types";

import { rich } from "./rich";
import { useSettingsAccess, type useSystemSettings } from "./use-system-settings";

export type WebSearchCardProps = Pick<
  ReturnType<typeof useSystemSettings>,
  | "webSearch"
  | "setWebSearch"
  | "backend"
  | "setBackend"
  | "braveKeySet"
  | "braveKeyInput"
  | "setBraveKeyInput"
  | "tavilyKeySet"
  | "tavilyKeyInput"
  | "setTavilyKeyInput"
  | "savingTavilyKey"
  | "keenableKeySet"
  | "keenableKeyInput"
  | "setKeenableKeyInput"
  | "savingKeenableKey"
  | "proxyInput"
  | "setProxyInput"
  | "savingProxy"
  | "testing"
  | "loaded"
  | "saving"
  | "savingKey"
  | "braveNeedsKey"
  | "saveBraveKey"
  | "saveTavilyKey"
  | "saveKeenableKey"
  | "saveProxy"
  | "runTest"
> & { saveWebSearch: (patch: Partial<Settings>) => void };

export function WebSearchCard({
  webSearch,
  setWebSearch,
  backend,
  setBackend,
  braveKeySet,
  braveKeyInput,
  setBraveKeyInput,
  tavilyKeySet,
  tavilyKeyInput,
  setTavilyKeyInput,
  savingTavilyKey,
  keenableKeySet,
  keenableKeyInput,
  setKeenableKeyInput,
  savingKeenableKey,
  proxyInput,
  setProxyInput,
  savingProxy,
  testing,
  loaded,
  saving,
  savingKey,
  braveNeedsKey,
  saveBraveKey,
  saveTavilyKey,
  saveKeenableKey,
  saveProxy,
  runTest,
  saveWebSearch,
}: WebSearchCardProps) {
  const { canAdmin, editable } = useSettingsAccess(loaded);
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <SearchIcon className="size-4" />
          {t("Web search")}
        </CardTitle>
        <CardDescription>
          {rich(
            t(
              "This is the web search <b>master switch + source config</b>. Only when it is enabled can <b>each agent's config</b> separately choose whether to enable <b>web_search</b> (returns only titles/links/snippets without fetching bodies; fetching is handled by WebFetch). Web search does <b>not</b> go through the recording proxy and is independent of traffic capture.",
            ),
          )}
          <br />
          {rich(
            t(
              "Sources: <b>DuckDuckGo (ddgs)</b> (no key needed), <b>Brave (free tier)</b> (requires a Brave API Key), <b>Tavily</b> (requires a Tavily API Key) or <b>DeepSeek</b> (reuses the current LLM config). When the master switch is off, the per-agent web search switches are unavailable.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="web-search" className="text-sm font-normal text-muted-foreground">
            {webSearch
              ? t("Master switch on · can be enabled per agent config")
              : t("Off · agents cannot enable web search")}
          </Label>
          <Switch
            id="web-search"
            checked={webSearch}
            disabled={!editable || saving}
            onCheckedChange={(v) => {
              setWebSearch(v); // optimistic
              saveWebSearch({ web_search_enabled: v });
            }}
          />
        </div>

        {webSearch && (
          <div className="flex items-center justify-between gap-4">
            <Label className="text-sm font-normal text-muted-foreground">{t("Search source")}</Label>
            <Select
              value={backend}
              disabled={!editable || saving}
              onValueChange={(v) => {
                setBackend(v); // optimistic
                saveWebSearch({ web_search_backend: v });
              }}
            >
              <SelectTrigger className="w-48 shrink-0">
                <SelectValue placeholder={t("Choose a source")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ddgs">{t("DuckDuckGo (ddgs · free, no key)")}</SelectItem>
                <SelectItem value="brave-free">{t("Brave (free tier · requires key)")}</SelectItem>
                <SelectItem value="tavily">{t("Tavily (requires key)")}</SelectItem>
                <SelectItem value="keenable">{t("Keenable (keyless, optional key)")}</SelectItem>
                <SelectItem value="deepseek">{t("DeepSeek (official)")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {webSearch && backend === "deepseek" && (
          <div className="border-border/60 bg-muted/30 flex flex-col gap-2 rounded-md border p-3">
            <p className="text-sm font-medium">{t("DeepSeek official web search")}</p>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {rich(
                t(
                  "This source directly reuses the <b>currently active LLM config</b>. It therefore supports <b>official DeepSeek models only</b>, and that config <b>must use the anthropic protocol</b> — DeepSeek's OpenAI-protocol endpoint does not support server-side search. This source may stop working after switching LLM configs.",
                ),
              )}
            </p>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {rich(
                t(
                  "Unlike other sources, the search runs <b>server-side on DeepSeek</b>: each search consumes an extra model call (incurring token costs), search requests <b>do not go through the egress proxy above</b> and are <b>not recorded as traffic</b>; results contain <b>titles and links only</b> (no snippets); use WebFetch when you need the page body.",
                ),
              )}
            </p>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {t(
                'Verify these conditions yourself; the system does not block on them. Use the "Test search" button below to run a real search and check.',
              )}
            </p>
          </div>
        )}

        {webSearch && backend === "brave-free" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="brave-key" className="text-sm font-normal text-muted-foreground">
              Brave Search API Key
              {braveKeySet && <span className="ml-2 text-xs text-emerald-500">{t("Configured")}</span>}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="brave-key"
                type="password"
                autoComplete="off"
                placeholder={braveKeySet ? t("Configured (leave empty to keep)") : t("Enter Brave API Key")}
                value={braveKeyInput}
                disabled={!editable || savingKey}
                onChange={(e) => setBraveKeyInput(e.target.value)}
              />
              {canAdmin && (
                <Button
                  type="button"
                  onClick={saveBraveKey}
                  disabled={!editable || savingKey || braveKeyInput.trim() === ""}
                >
                  {t("Save")}
                </Button>
              )}
            </div>
            {braveNeedsKey && (
              <p className="text-xs text-amber-500">
                {t("Brave is selected but no key is configured — the search tool stays disabled until you save one.")}
              </p>
            )}
            <p className="text-muted-foreground text-xs">
              {t("The free tier allows about 2,000 queries/month. Get a key at https://brave.com/search/api/.")}
            </p>
          </div>
        )}

        {webSearch && backend === "tavily" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="tavily-key" className="text-sm font-normal text-muted-foreground">
              Tavily Search API Key
              {tavilyKeySet && <span className="ml-2 text-xs text-emerald-500">{t("Configured")}</span>}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="tavily-key"
                type="password"
                autoComplete="off"
                placeholder={tavilyKeySet ? t("Configured (leave empty to keep)") : t("Enter Tavily API Key (tvly-…)")}
                value={tavilyKeyInput}
                disabled={!editable || savingTavilyKey}
                onChange={(e) => setTavilyKeyInput(e.target.value)}
              />
              {canAdmin && (
                <Button
                  type="button"
                  onClick={saveTavilyKey}
                  disabled={!editable || savingTavilyKey || tavilyKeyInput.trim() === ""}
                >
                  {t("Save")}
                </Button>
              )}
            </div>
            {webSearch && backend === "tavily" && !tavilyKeySet && (
              <p className="text-xs text-amber-500">
                {t("Tavily is selected but no key is configured — the search tool stays disabled until you save one.")}
              </p>
            )}
            <p className="text-muted-foreground text-xs">{t("Register and get an API key at https://tavily.com.")}</p>
          </div>
        )}

        {webSearch && backend === "keenable" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="keenable-key" className="text-sm font-normal text-muted-foreground">
              {t("Keenable Search API Key (optional)")}
              {keenableKeySet && <span className="ml-2 text-xs text-emerald-500">{t("Configured")}</span>}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="keenable-key"
                type="password"
                autoComplete="off"
                placeholder={
                  keenableKeySet
                    ? t("Configured (leave empty to keep)")
                    : t("Optional: leave empty for the keyless endpoint")
                }
                value={keenableKeyInput}
                disabled={!editable || savingKeenableKey}
                onChange={(e) => setKeenableKeyInput(e.target.value)}
              />
              {canAdmin && (
                <Button
                  type="button"
                  onClick={saveKeenableKey}
                  disabled={!editable || savingKeenableKey || keenableKeyInput.trim() === ""}
                >
                  {t("Save")}
                </Button>
              )}
            </div>
            <p className="text-muted-foreground text-xs leading-relaxed">
              {rich(
                t(
                  "Queries leave this instance and go to api.keenable.ai. The request carries <b>X-Keenable-Title: ARTEX</b> so the provider can identify the caller. With no key, the public endpoint is rate-limited per IP; a key raises the limit, but the backend works without one.",
                ),
              )}
            </p>
          </div>
        )}

        {webSearch && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="ws-proxy" className="text-sm font-normal text-muted-foreground">
              {t("Egress proxy (optional)")}
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="ws-proxy"
                autoComplete="off"
                placeholder={t("http://host:port or socks5://host:port (empty = direct)")}
                value={proxyInput}
                disabled={!editable || savingProxy}
                onChange={(e) => setProxyInput(e.target.value)}
              />
              {canAdmin && (
                <Button type="button" onClick={saveProxy} disabled={!editable || savingProxy}>
                  {t("Save")}
                </Button>
              )}
            </div>
            <p className="text-muted-foreground text-xs">
              {t(
                "A dedicated egress proxy used only to reach search endpoints (VPN/SOCKS etc.). Unrelated to the traffic-recording MITM proxy; use it to reach endpoints when the network is blocked.",
              )}
            </p>
          </div>
        )}

        {webSearch && (
          <div className="flex items-center justify-between gap-4 border-t pt-4">
            <p className="text-muted-foreground text-xs">
              {t('Run a real "test" search with the current config (source + proxy + key) to verify it works.')}
            </p>
            {canAdmin && (
              <Button
                type="button"
                variant="outline"
                onClick={runTest}
                disabled={!editable || testing}
                className="shrink-0"
              >
                {testing ? t("Testing…") : t("Test search")}
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
