import type { TaskConstraint } from "./types";

// Origins that count as the operator; mirrors operatorGoalOrigins in
// agent/untrusted_provenance.go. Any other origin (the main agent's
// set_constraints writes "mainagent") is an agent proposal: the planner and
// workers see it as data, and an allow grants nothing until the operator adopts
// it (an operator edit, including "Adopt", records origin "human").
const OPERATOR_ORIGINS: ReadonlySet<string> = new Set(["human", "goals", "system", ""]);

export function isAgentProposed(c: Pick<TaskConstraint, "origin">): boolean {
  return !OPERATOR_ORIGINS.has(c.origin ?? "");
}
