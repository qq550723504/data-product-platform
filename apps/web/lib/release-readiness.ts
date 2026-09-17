/** Validate Core's readiness response; never calculate or replace Core readiness. */
export const requiredReleaseGates = [
  "production", "dataset", "rights", "quality", "compliance", "contract", "evidence", "delivery",
] as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** null means the response is complete and internally consistent, NOT authorization. */
export function releaseReadinessProblem(value: unknown): string | null {
  if (!isRecord(value) || value.overall !== "READY") {
    return "Core 尚未将 Readiness 判定为 READY，请刷新后核对。";
  }
  const checks = value.checks;
  if (!isRecord(checks)) return "Readiness 检查项缺失或格式无效，已阻止发布。";
  const missing = requiredReleaseGates.filter((gate) => !Object.hasOwn(checks, gate));
  if (missing.length) return `Readiness Gate 缺失：${missing.join("、")}；已阻止发布。`;
  // Additional gates introduced by Core must also pass; do not silently ignore them.
  const failed = Object.entries(checks).filter(([, status]) => status !== "PASS").map(([gate]) => gate);
  if (failed.length) return `Readiness Gate 未通过：${failed.join("、")}；已阻止发布。`;
  if (!Array.isArray(value.blockers) || !value.blockers.every((item) => typeof item === "string")) {
    return "Readiness 阻塞原因缺失或格式无效，已阻止发布。";
  }
  if (value.blockers.length) return `Core 仍返回阻塞原因：${value.blockers.join("、")}；已阻止发布。`;
  return null;
}
