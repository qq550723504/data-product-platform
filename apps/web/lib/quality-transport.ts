// Only server-side Quality clients supply this credential. It must never be
// attached to unrelated reads, including the DatasetVersion precheck.
export function qualityHeaders(token: string | null | undefined): Record<string, string> {
  const credential = token?.trim();
  if (!credential) throw new Error("受信任的 Quality 凭据未配置，证据状态 UNKNOWN。");
  return { Accept: "application/json", Authorization: `Bearer ${credential}` };
}

export class QualityReadError extends Error {
  constructor(readonly status: number) { super(`Quality 证据不可确认（HTTP ${status}），状态 UNKNOWN。`); }
}

export async function qualityGet<T>(path: string, base: string, token: string | null | undefined, request: typeof fetch = fetch): Promise<T> {
  const response = await request(`${base.replace(/\/$/, "")}${path}`, {
    cache: "no-store", redirect: "error", signal: AbortSignal.timeout(15000), headers: qualityHeaders(token),
  });
  // Do not copy upstream bodies or credentials into UI errors.
  if (!response.ok) throw new QualityReadError(response.status);
  return await response.json() as T;
}
