export const requiredReadinessGates = [
  "production",
  "dataset",
  "rights",
  "quality",
  "compliance",
  "contract",
  "evidence",
  "delivery",
] as const;

export type RequiredReadinessGate = (typeof requiredReadinessGates)[number];

export type ReleaseReadinessEvaluation = {
  ready: boolean;
  problems: string[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

/**
 * Fail-closed runtime validation shared by the browser rendering path and the
 * server-side publish preflight. Typed Core responses still cross an HTTP
 * boundary, so compile-time `ReleaseReadiness` types are not sufficient here.
 */
export function evaluateReleaseReadiness(releaseStatus: unknown, readiness: unknown): ReleaseReadinessEvaluation {
  const problems: string[] = [];
  if (releaseStatus !== "READY") {
    problems.push("RELEASE_NOT_READY");
  }
  if (!isRecord(readiness)) {
    return { ready: false, problems: [...problems, "READINESS_INVALID"] };
  }
  if (readiness.overall !== "READY") {
    problems.push("READINESS_NOT_READY");
  }

  if (!Array.isArray(readiness.blockers) || readiness.blockers.some((value) => typeof value !== "string")) {
    problems.push("BLOCKERS_INVALID");
  } else if (readiness.blockers.length > 0) {
    problems.push("BLOCKERS_PRESENT");
  }

  if (!isRecord(readiness.checks)) {
    problems.push("CHECKS_INVALID");
    return { ready: false, problems };
  }

  const checks = readiness.checks;
  for (const gate of requiredReadinessGates) {
    if (!(gate in checks)) {
      problems.push(`GATE_MISSING:${gate}`);
    } else if (checks[gate] !== "PASS") {
      problems.push(`GATE_NOT_PASS:${gate}`);
    }
  }

  // Future Core gates fail closed unless they are explicitly PASS. This avoids
  // silently publishing when the Core adds a new release condition that an
  // older web build does not yet render as one of the eight canonical gates.
  for (const [gate, status] of Object.entries(checks)) {
    if (!requiredReadinessGates.includes(gate as RequiredReadinessGate) && status !== "PASS") {
      problems.push(`EXTRA_GATE_NOT_PASS:${gate}`);
    }
  }

  return { ready: problems.length === 0, problems };
}
