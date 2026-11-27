// Real backend client. /api/* is proxied to the Go backend (next.config rewrites).
// Composed from the per-domain modules in ./api/; the exported `api` shape is unchanged.

export { ApiError, http } from "./api/core";

import { agentsApi } from "./api/agents";
import { assetsApi } from "./api/assets";
import { explorationApi } from "./api/exploration";
import { interceptApi } from "./api/intercept";
import { llmApi } from "./api/llm";
import { recordsApi } from "./api/records";
import { settingsApi } from "./api/settings";
import { tasksApi } from "./api/tasks";
import { toolsApi } from "./api/tools";
import { trafficApi } from "./api/traffic";

export const api = {
  ...tasksApi,
  ...assetsApi,
  ...explorationApi,
  ...trafficApi,
  ...settingsApi,
  ...llmApi,
  ...agentsApi,
  ...toolsApi,
  ...interceptApi,
  ...recordsApi,
};
