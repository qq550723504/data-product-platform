import Link from "next/link";

export function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        {eyebrow ? <span className="eyebrow">{eyebrow}</span> : null}
        <h1>{title}</h1>
        {description ? <p>{description}</p> : null}
      </div>
      {action ? <div className="page-header-action">{action}</div> : null}
    </div>
  );
}

export function Badge({ value, tone }: { value?: string | null; tone?: "neutral" | "good" | "warn" | "bad" | "info" }) {
  const normalized = (value ?? "UNKNOWN").trim() || "UNKNOWN";
  const inferred = tone ?? badgeTone(normalized);
  return <span className={`badge badge-${inferred}`}>{normalized}</span>;
}

function badgeTone(value: string): "neutral" | "good" | "warn" | "bad" | "info" {
  switch (value.toUpperCase()) {
    case "READY":
    case "ACTIVE":
    case "SUCCEEDED":
    case "PUBLISHED":
    case "PASS":
    case "APPROVED":
    case "CONFIRMED":
    case "AUTO_CONFIRMED":
    case "HEALTHY":
      return "good";
    case "RUNNING":
    case "SUBMITTING":
    case "REVIEW":
    case "PENDING":
    case "WAITING_REVIEW":
      return "info";
    case "DRAFT":
    case "QUEUED":
    case "UNRESOLVED":
    case "SUPERSEDED":
      return "warn";
    case "FAILED":
    case "REJECTED":
    case "INVALIDATED":
    case "CONFLICT":
    case "UNHEALTHY":
      return "bad";
    default:
      return "neutral";
  }
}

export function MetricCard({ label, value, hint }: { label: string; value: string | number; hint?: string }) {
  return (
    <article className="metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
      {hint ? <small>{hint}</small> : null}
    </article>
  );
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="empty-state">
      <div className="empty-glyph">·</div>
      <strong>{title}</strong>
      <p>{description}</p>
    </div>
  );
}

export function SetupRequired() {
  return (
    <div className="callout callout-warn">
      <strong>尚未配置 POC Workspace</strong>
      <p>
        在 <code>apps/web/.env.local</code> 设置 <code>POC_WORKSPACE_ID</code>，控制台就能从 Core Read Model 自动发现资源、数据集、执行记录与数据产品。
      </p>
    </div>
  );
}

export function LoadError({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : "加载 Core 数据失败";
  return (
    <div className="callout callout-bad">
      <strong>读取失败</strong>
      <p>{message}</p>
    </div>
  );
}

export function BackLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link href={href} className="back-link">
      ← {children}
    </Link>
  );
}

export function DefinitionList({ items }: { items: Array<{ label: string; value: React.ReactNode }> }) {
  return (
    <dl className="definition-list">
      {items.map((item) => (
        <div key={item.label}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function formatDate(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

export function shortId(value?: string | null): string {
  if (!value) return "—";
  return value.length > 13 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}

export function formatBytes(value?: number | null): string {
  if (value === undefined || value === null) return "—";
  if (value < 1024) return `${value} B`;
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`;
  return `${(value / 1024 ** 3).toFixed(1)} GB`;
}
