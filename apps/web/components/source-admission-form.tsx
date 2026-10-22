"use client";
import { useEffect, useState } from "react";
import { admissionText, type AdmissionLocale } from "@/lib/source-admission-text";
import styles from "./source-admission-form.module.css";

type Intent = { key: string; releaseId: string; snapshotId: string; manifestSha256: string };
type Receipt = { operationId: string; datasetVersionId?: string; state: string; rowCount: number };
const empty = { releaseId: "", snapshotId: "", manifestSha256: "" };
export function SourceAdmissionForm({ lang, scope }: { lang: AdmissionLocale; scope: string }) {
  const t = admissionText[lang];
  const storage = `fork-approved-source:${scope}`;
  const [fields, setFields] = useState(empty);
  const [intent, setIntent] = useState<Intent | null>(null);
  const [receipt, setReceipt] = useState<Receipt | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [blocked, setBlocked] = useState(false);
  useEffect(() => { try { const saved = sessionStorage.getItem(storage); if (saved) { const i = JSON.parse(saved) as Intent; if (i.key && i.releaseId && i.snapshotId && i.manifestSha256) { setIntent(i); setFields(i); } } } catch {} }, [storage]);
  function problem(code: string) {
    setReceipt(null);
    if (code === "SOURCE_AUTHORITY_DENIED") { setBlocked(true); return t.denied; }
    if (code === "SOURCE_INTEGRITY_FAILED") return t.integrity;
    if (code === "SOURCE_IDEMPOTENCY_CONFLICT") return t.conflict;
    if (code === "SOURCE_PROFILE_UNSUPPORTED") return t.unsupported;
    return t.unknown;
  }
  async function run(lookup: boolean) {
    const i = intent ?? { ...fields, key: crypto.randomUUID() };
    if (!intent) { setIntent(i); sessionStorage.setItem(storage, JSON.stringify(i)); }
    setBusy(true); setMessage(""); setReceipt(null);
    try {
      const r = await fetch(`/api/source-admissions${lookup ? `?key=${encodeURIComponent(i.key)}` : ""}`, { method: lookup ? "GET" : "POST", cache: "no-store", headers: { "Content-Type": "application/json", "Idempotency-Key": i.key }, ...(lookup ? {} : { body: JSON.stringify({ releaseId: i.releaseId, snapshotId: i.snapshotId, manifestSha256: i.manifestSha256 }) }) });
      const data = await r.json();
      if (!r.ok) { setMessage(problem(data.error?.code)); return; }
      if (data.state === "FAILED_FINAL") { setBlocked(true); setMessage(t.failed); return; }
      setReceipt(data); setMessage(data.state === "ACCEPTED_SOURCE" ? t.accepted : t.prepared);
    } catch { setMessage(problem("SOURCE_TRANSPORT_UNKNOWN")); }
    finally { setBusy(false); }
  }
  return <div className="detail-card">
    <form className={styles.form} onSubmit={(e) => { e.preventDefault(); void run(false); }}>
      {(["releaseId", "snapshotId", "manifestSha256"] as const).map((key) => <label className={styles.field} key={key}>{key === "releaseId" ? t.release : key === "snapshotId" ? t.snapshot : t.hash}<input name={key} required value={fields[key]} disabled={Boolean(intent) || busy} onChange={(e) => setFields({ ...fields, [key]: e.target.value })} pattern={key === "manifestSha256" ? "[0-9a-f]{64}" : "[0-9a-fA-F-]{36}"} /></label>)}
      <button className={`${styles.button} ${styles.primary}`} disabled={busy || blocked}>{busy ? t.busy : t.submit}</button>
    </form>
    {intent && <><p className={styles.identity}>{t.key}: <code data-testid="request-key">{intent.key}</code></p><div className={styles.actions}><button className={styles.button} disabled={busy} onClick={() => void run(true)}>{t.lookup}</button><button className={styles.button} disabled={busy} onClick={() => { sessionStorage.removeItem(storage); setIntent(null); setFields(empty); setReceipt(null); setMessage(""); setBlocked(false); }}>{t.fresh}</button></div></>}
    {message && <p className={styles.result} role="status">{message}</p>}
    {receipt?.state === "ACCEPTED_SOURCE" && <dl className={styles.facts}><dt>{t.version}</dt><dd data-testid="accepted-version">{receipt.datasetVersionId}</dd><dt>{t.count}</dt><dd>{receipt.rowCount}</dd></dl>}
  </div>;
}
