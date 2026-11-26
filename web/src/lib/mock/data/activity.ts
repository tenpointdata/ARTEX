import type { Activity } from "@/lib/types";

import { activityPlanner } from "./activity-planner";
import { activityWorkA } from "./activity-work-a";
import { activityWorkB } from "./activity-work-b";
import { activityWorkC } from "./activity-work-c";

export const activity: Activity[] = [...activityPlanner, ...activityWorkA, ...activityWorkB, ...activityWorkC];

export function activityForTask(): Activity[] {
  return activity;
}
