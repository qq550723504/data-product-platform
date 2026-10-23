import "server-only";
import { qualityGet } from "./quality-transport";

export function qualityRead<T>(path: string): Promise<T> {
  return qualityGet<T>(path, process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080", process.env.HUMAN_DECISION_API_TOKEN);
}
