import "server-only";
import { configuredWorkspaceId } from "./platform";
import { agentContextGet, type AgentSelection } from "./agent-quality";

export function readAgentContext(selection: AgentSelection) {
  return agentContextGet(
    configuredWorkspaceId() ?? "",
    selection,
    process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
    process.env.HUMAN_DECISION_API_TOKEN,
  );
}
