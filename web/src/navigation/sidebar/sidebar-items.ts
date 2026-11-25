import {
  Activity,
  Ban,
  BellRing,
  Bot,
  Brain,
  Coins,
  Bug,
  Cable,
  ClipboardList,
  FileUp,
  FileText,
  FolderOpen,
  FolderSync,
  LayoutDashboard,
  type LucideIcon,
  MessageSquare,
  Network,
  Plug,
  Radio,
  ScrollText,
  Settings2,
  ShieldAlert,
  Sparkles,
  Target,
  Terminal,
  Users,
  Wrench,
} from "lucide-react";

import { t } from "@/i18n";
import type { Permission } from "@/lib/governance-types";

export type NavBadge = "new" | "soon";

export interface NavSubItem {
  id: string;
  title: string;
  url: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
  // need is the permission the page requires; the sidebar hides it otherwise.
  // Unset means "read" (server/rbac_policy.go: a GET needs read by default).
  need?: Permission;
  // hideWith hides the item from sessions holding that permission: a page
  // that stands in for one they already have (the client summary for Tasks).
  hideWith?: Permission;
}

interface NavItemBase {
  id: string;
  title: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
  need?: Permission;
  hideWith?: Permission;
}

export interface NavMainLinkItem extends NavItemBase {
  url: string;
  subItems?: never;
}

export interface NavMainParentItem extends NavItemBase {
  subItems: NavSubItem[];
}

export type NavMainItem = NavMainLinkItem | NavMainParentItem;

export interface NavGroup {
  id: number;
  label?: string;
  items: NavMainItem[];
}

export const sidebarItems: NavGroup[] = [
  {
    id: 1,
    label: t("Functions"),
    items: [
      { id: "dashboard", title: t("Dashboard"), url: "/dashboard", icon: LayoutDashboard },
      { id: "chat", title: t("Chat"), url: "/chat", icon: MessageSquare, need: "operate" },
      { id: "tasks", title: t("Tasks"), url: "/function/tasks", icon: Target },
      {
        id: "reports",
        title: t("Reports"),
        url: "/function/reports",
        icon: FileText,
        need: "client_read",
        hideWith: "read",
      },
      { id: "findings", title: t("Findings"), url: "/function/findings", icon: Bug, need: "client_read" },
      { id: "traffic", title: t("Traffic"), url: "/function/traffic", icon: Activity },
      { id: "commands", title: t("Tool Execution"), url: "/function/commands", icon: Terminal },
      { id: "llm-records", title: t("LLM Recording"), url: "/function/llm-records", icon: Radio, need: "operate" },
      { id: "cost", title: t("Cost & efficiency"), url: "/function/cost", icon: Coins },
      { id: "assets", title: t("Assets"), url: "/function/assets", icon: Network },
      { id: "sync", title: t("Asset Sync"), url: "/function/sync", icon: FolderSync },
      { id: "workspace", title: t("Workspace"), url: "/function/workspace", icon: FolderOpen, need: "operate" },
    ],
  },
  {
    id: 2,
    label: t("System"),
    items: [
      { id: "llm", title: t("LLM"), url: "/system/llm", icon: Brain },
      { id: "agents", title: t("Agent"), url: "/system/agents", icon: Bot },
      { id: "mcp", title: t("MCP"), url: "/system/mcp", icon: Plug },
      { id: "skills", title: t("Skill"), url: "/system/skills", icon: Sparkles },
      { id: "tools", title: t("Tools"), url: "/system/tools", icon: Wrench },
      { id: "notify", title: t("Notifications"), url: "/system/notify", icon: BellRing },
      { id: "integrations", title: t("Integrations"), url: "/system/integrations", icon: Cable },
      { id: "imports", title: t("Scanner imports"), url: "/system/integrations/imports", icon: FileUp },
      { id: "intercept", title: t("Interception Rules"), url: "/system/intercept", icon: ShieldAlert },
      { id: "asset-intercept", title: t("Asset Interception"), url: "/system/intercept/assets", icon: Ban },
      { id: "approvals", title: t("Approval Records"), url: "/system/intercept/approvals", icon: ClipboardList },
      { id: "users", title: t("Users"), url: "/system/users", icon: Users, need: "admin" },
      { id: "logs", title: t("Logs"), url: "/system/logs", icon: ScrollText, need: "operate" },
      { id: "settings", title: t("System Settings"), url: "/system/settings", icon: Settings2 },
    ],
  },
];

// navFor keeps the items (and sub-items) the session's permissions allow, and
// drops groups left empty.
export function navFor(groups: NavGroup[], allowed: (p: Permission) => boolean): NavGroup[] {
  const ok = (x: { need?: Permission; hideWith?: Permission }) =>
    allowed(x.need ?? "read") && !(x.hideWith && allowed(x.hideWith));
  return groups
    .map((g) => ({
      ...g,
      items: g.items.filter(ok).map((it) => (it.subItems ? { ...it, subItems: it.subItems.filter(ok) } : it)),
    }))
    .filter((g) => g.items.length > 0);
}
