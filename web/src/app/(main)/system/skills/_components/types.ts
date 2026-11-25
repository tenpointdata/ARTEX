export interface TreeNode {
  name: string;
  path: string; // relative to skill root, dirs WITHOUT trailing slash
  type: "file" | "dir";
  children: TreeNode[];
}

export type Selected = { skill: string; path: null } | { skill: string; path: string };

export type Creating = {
  skill: string;
  inDir: string;
  kind: "file" | "dir";
} | null;

export type PendingDelete =
  | { kind: "skill"; skill: string }
  | { kind: "file"; skill: string; path: string }
  | { kind: "dir"; skill: string; path: string }
  | null;
