import Link from "next/link";
import { paginationWindow } from "@/lib/pagination";
import { businessText, displayText } from "../lib/business-language.mjs";

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
        {displayText(eyebrow ? <span className="eyebrow">{businessText(eyebrow)}</span> : null)}
        <h1>{businessText(title)}</h1>
        {displayText(description ? <p>{businessText(description)}</p> : null)}
      </div>
      {action ? <div className="page-header-action">{displayText(action)}</div> : null}
    </div>
  );
}

export function Badge({ value, tone }: { value?: string | null; tone?: "neutral" | "good" | "warn" | "bad" | "info" }) {
  const normalized = (value ?? "UNKNOWN").trim() || "UNKNOWN";
  const inferred = tone ?? badgeTone(normalized);
  return <span className={`badge badge-${inferred}`} data-status={normalized}>{businessText(normalized)}</span>;
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
    case "CERTIFIED":
    case "ALLOWED":
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
    case "PASS_WITH_WARNING":
    case "NOT_CERTIFIED":
      return "warn";
    case "FAILED":
    case "FAIL":
    case "REJECTED":
    case "INVALIDATED":
    case "CONFLICT":
    case "UNHEALTHY":
    case "BLOCKED":
    case "REVOKED":
      return "bad";
    default:
      return "neutral";
  }
}

export function MetricCard({ label, value, hint }: { label: string; value: string | number; hint?: string }) {
  return (
    <article className="metric-card">
      <span>{businessText(label)}</span>
      <strong>{businessText(value)}</strong>
      {displayText(hint ? <small>{businessText(hint)}</small> : null)}
    </article>
  );
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="empty-state">
      <div className="empty-glyph">·</div>
      <strong>{businessText(title)}</strong>
      <p>{businessText(description)}</p>
    </div>
  );
}

export function SetupRequired() {
  return (
    <div className="callout callout-warn">
      <strong>工作空间尚未配置</strong>
      <p>
        请联系管理员完成工作空间接入，以查询资源、数据集、执行记录和数据产品。
      </p>
    </div>
  );
}

export function LoadError({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : "加载数据失败";
  return (
    <div className="callout callout-bad">
      <strong>读取失败</strong>
      <p>{displayText(/不属于|无权|权限|不可见|未配置|未找到|无效|越界/.test(message)?businessText(message):'当前记录暂时无法读取，请重试或联系管理员。')}</p><details><summary>查看错误详情</summary><code>{message}</code></details>
    </div>
  );
}

export function BackLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link href={href} className="back-link">
      ← {displayText(children)}
    </Link>
  );
}

export function Pagination({
  basePath,
  offset,
  itemCount,
  total,
  pageSize,
  label = "列表分页",
}: {
  basePath: string;
  offset: number;
  itemCount: number;
  total: number;
  pageSize: number;
  label?: string;
}) {
  const view = paginationWindow(offset, itemCount, total, pageSize);
  return (
    <nav aria-label={label} className="pagination">
      {displayText(view.previousOffset === null ? (
        <span className="pagination-step pagination-step-disabled" aria-disabled="true">上一页</span>
      ) : (
        <Link className="pagination-step" rel="prev" href={`${basePath}?offset=${view.previousOffset}`}>上一页</Link>
      ))}
      <span className="pagination-summary">{displayText(view.summary)}</span>
      {displayText(view.nextOffset === null ? (
        <span className="pagination-step pagination-step-disabled" aria-disabled="true">下一页</span>
      ) : (
        <Link className="pagination-step" rel="next" href={`${basePath}?offset=${view.nextOffset}`}>下一页</Link>
      ))}
    </nav>
  );
}

export function DefinitionList({ items }: { items: Array<{ label: string; value: React.ReactNode }> }) {
  return (
    <dl className="definition-list">
      {items.map((item) => (
        <div key={item.label}>
          <dt>{businessText(item.label)}</dt>
          <dd>{displayText(typeof item.value==='string'?businessText(item.value):item.value)}</dd>
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
