"use client";

import { RadioTowerIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";

import { rich } from "./rich";

export function TrafficCaptureCard({
  trafficCapture,
  loaded,
  saving,
  onToggle,
}: {
  trafficCapture: boolean;
  loaded: boolean;
  saving: boolean;
  onToggle: (v: boolean) => void;
}) {
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <RadioTowerIcon className="size-4" />
          {t("Traffic capture")}
        </CardTitle>
        <CardDescription>
          {t(
            "When enabled, every agent's HTTP traffic is stored in full through the recording proxy, and agents are injected with the traffic_search / traffic_get tools and proxy configuration (the prompt includes proxy notes).",
          )}
          <br />
          {rich(
            t(
              "When off (default) no traffic is recorded: the agent does <b>not</b> receive proxy configuration or traffic tools, and the prompt contains <b>no</b> proxy-related content. Agents are rebuilt immediately after switching.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-2.5">
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="traffic-capture" className="text-sm font-normal text-muted-foreground">
            {trafficCapture
              ? t("On · recording traffic and injecting proxy")
              : t("Off · no recording, no proxy injection")}
          </Label>
          <Switch
            id="traffic-capture"
            checked={trafficCapture}
            disabled={!loaded || saving}
            onCheckedChange={onToggle}
          />
        </div>
        {loaded && !trafficCapture && (
          <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
            {t(
              "Traffic capture is off: findings stay recorded, but captured HTTP traffic is required for provable findings.",
            )}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function AgentTrafficBindingCard({
  agentTrafficBinding,
  loaded,
  saving,
  onToggle,
}: {
  agentTrafficBinding: boolean;
  loaded: boolean;
  saving: boolean;
  onToggle: (v: boolean) => void;
}) {
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <RadioTowerIcon className="size-4" />
          {t("Automatic agent traffic binding")}
        </CardTitle>
        <CardDescription id="agent-traffic-binding-description">
          {rich(
            t(
              "Off by default. When enabled, the report agent triggered on vulnerability creation reviews existing HTTP requests/responses and links the matching traffic before writing the report. <b>Reading packets and the extra tool calls increase token consumption.</b>",
            ),
          )}
          <br />
          {t(
            "Reporting still works for TCP, uncaptured traffic, or when no traffic matches. This switch does not affect traffic capture, manual binding, or viewing saved evidence. It takes effect for the next round of agents; once disabled, new automatic bindings are rejected immediately.",
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-4">
        <Label htmlFor="agent-traffic-binding" className="text-sm font-normal text-muted-foreground">
          {agentTrafficBinding ? t("On · increases token consumption") : t("Off · manual binding still available")}
        </Label>
        <Switch
          id="agent-traffic-binding"
          aria-describedby="agent-traffic-binding-description"
          checked={agentTrafficBinding}
          disabled={!loaded || saving}
          onCheckedChange={onToggle}
        />
      </CardContent>
    </Card>
  );
}

export function GlobalProxyCard({
  globalProxyInput,
  setGlobalProxyInput,
  savingGlobalProxy,
  loaded,
  saveGlobalProxy,
}: {
  globalProxyInput: string;
  setGlobalProxyInput: (v: string) => void;
  savingGlobalProxy: boolean;
  loaded: boolean;
  saveGlobalProxy: () => void;
}) {
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <RadioTowerIcon className="size-4" />
          {t("Global proxy")}
        </CardTitle>
        <CardDescription>
          {rich(
            t(
              "All agents' <b>target traffic</b> egresses through this proxy (hide the source IP / use a jump host). Supports <b>http / https / socks5</b>, optionally with <code>user:pass</code> authentication. Empty = direct connection.",
            ),
          )}
          <br />
          {rich(
            t(
              "With <b>traffic capture</b> on, it acts as the recording proxy's <b>upstream</b> (traffic is still stored in full, then egresses through this proxy); with capture off, it is injected directly into the agent's bash / WebFetch egress. Independent of the web search proxy and the LLM proxy.",
            ),
          )}
          <br />
          {rich(
            t(
              "<b>Note</b>: with <b>capture off</b>, socks5 relies on each CLI tool honoring <code>ALL_PROXY</code> (curl works; some tools may ignore it). If you mainly use socks5, enable traffic capture — on that path the MITM does the dialing itself, invisible to tools and reliably effective.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <Label htmlFor="global-proxy" className="text-sm font-normal text-muted-foreground">
          {t("Proxy address")}
        </Label>
        <div className="flex items-center gap-2">
          <Input
            id="global-proxy"
            autoComplete="off"
            placeholder={t("socks5://user:pass@host:1080 or http://host:port (empty = direct)")}
            value={globalProxyInput}
            disabled={!loaded || savingGlobalProxy}
            onChange={(e) => setGlobalProxyInput(e.target.value)}
          />
          <Button type="button" onClick={saveGlobalProxy} disabled={!loaded || savingGlobalProxy}>
            {t("Save")}
          </Button>
        </div>
        <p className="text-muted-foreground text-xs">
          {globalProxyInput.trim()
            ? t("Configured · all target traffic egresses through this proxy")
            : t("Not configured · target traffic egresses directly")}
        </p>
      </CardContent>
    </Card>
  );
}
