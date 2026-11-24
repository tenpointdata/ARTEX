import {
  Activity,
  Ban,
  BellRing,
  Bot,
  Brain,
  Bug,
  ClipboardList,
  Coins,
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
  Wrench,
} from "lucide-react";

import { t } from "@/i18n";
import type { PermissionGated } from "@/lib/permissions";

export type NavBadge = "new" | "soon";

// `requires` hides an entry from roles without that permission (the server
// refuses the route anyway; this keeps the menu to what the role can open).
export interface NavSubItem extends PermissionGated {
  id: string;
  title: string;
  url: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
}

interface NavItemBase extends PermissionGated {
  id: string;
  title: string;
  icon?: LucideIcon;
  badge?: NavBadge;
  disabled?: boolean;
  newTab?: boolean;
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
      { id: "chat", title: t("Chat"), url: "/chat", icon: MessageSquare },
      { id: "tasks", title: t("Tasks"), url: "/function/tasks", icon: Target },
      { id: "findings", title: t("Findings"), url: "/function/findings", icon: Bug },
      { id: "traffic", title: t("Traffic"), url: "/function/traffic", icon: Activity },
      { id: "commands", title: t("Tool Execution"), url: "/function/commands", icon: Terminal },
      { id: "llm-records", title: t("LLM Recording"), url: "/function/llm-records", icon: Radio },
      // Global spend across every company: not for client_read (c-9ca57a).
      { id: "cost", title: t("Cost & efficiency"), url: "/function/cost", icon: Coins, requires: "read" },
      { id: "assets", title: t("Assets"), url: "/function/assets", icon: Network },
      { id: "sync", title: t("Asset Sync"), url: "/function/sync", icon: FolderSync },
      { id: "workspace", title: t("Workspace"), url: "/function/workspace", icon: FolderOpen },
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
      { id: "intercept", title: t("Interception Rules"), url: "/system/intercept", icon: ShieldAlert },
      { id: "asset-intercept", title: t("Asset Interception"), url: "/system/intercept/assets", icon: Ban },
      { id: "approvals", title: t("Approval Records"), url: "/system/intercept/approvals", icon: ClipboardList },
      { id: "logs", title: t("Logs"), url: "/system/logs", icon: ScrollText },
      { id: "settings", title: t("System Settings"), url: "/system/settings", icon: Settings2 },
    ],
  },
];
