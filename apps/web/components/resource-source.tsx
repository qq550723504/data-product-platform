import Link from "next/link";
import { Badge, DefinitionList, formatDate } from "./ui";
import { CommandButton } from "./batch-controls";
import { batchEnabled, batches, core, source, workspace } from "@/lib/batches";
import { refreshMetadata } from "@/app/batches/actions";

type PartyReference = { id: string; type: string; name: string; displayName: string };
type MetadataObservation = {
  status: string;
  provider?: string;
  externalId?: string;
  externalFqn?: string;
  version?: string | number;
  lastConfirmedVersion?: string | number;
  lastConfirmedAt?: string;
  observedAt?: string;
  governance?: {
    description?: string;
    owners?: PartyReference[];
    stewards?: PartyReference[];
    classifications?: string[];
    terms?: string[];
  } | null;
};

function partyNames(parties?: PartyReference[]) {
  return parties?.map(p => `${p.displayName || p.name || p.id} (${p.type})`).join("、");
}

export async function ResourceSource({ id }: { id: string }) {
  if (!batchEnabled()) return null;
  try {
    const [items, s] = await Promise.all([batches(), source()]);
    const linked = items.filter(b => b.request.slots.some(x => x.resourceId === id));
    if (!linked.length) return null;
    const slot = linked[0].request.slots.find(x => x.resourceId === id)!;
    const table = s.tables.find(t => t.Code === slot.code);
    const m = await core<MetadataObservation>(
      `/data-resources/${id}/metadata-observation?workspaceId=${workspace()}`,
      "GET", undefined, undefined, true,
    );
    // A failed current observation cannot label cached annotations as current,
    // or confuse unavailable fields with an explicitly unassigned Steward.
    const g = m.status === "OBSERVED" ? m.governance : null;
    const missing = m.status === "OBSERVED" && g ? "未指定" : "UNKNOWN（本次无法确认）";
    return (
      <section className="detail-card">
        <h2>PostgreSQL 来源与批次</h2>
        <p>{s.database}.{table?.Schema}.{table?.Name} · {s.connectionRef} · {s.instanceRef}</p>
        <p>只读账户 {s.reader} · 原始 schema：{slot.columns.join(", ")}</p>
        <h3>metadata 治理观察</h3>
        <Badge value={m.status} />
        <p>{m.status === "OBSERVED"
          ? "提供方已响应本次观察。"
          : "治理信息陈旧 / 未知；metadata 可能失联或尚未绑定。当前不能确认提供方状态。"}</p>
        <p>来源 {m.provider ?? "OpenMetadata（未确认）"} · 当前观察版本 {m.version ?? "UNKNOWN"} · 本次观察 {formatDate(m.observedAt)}</p>
        <p>绑定缓存版本 {m.lastConfirmedVersion ?? "UNKNOWN"} · 上次绑定确认 {formatDate(m.lastConfirmedAt)}</p>
        <p>外部对象 {m.externalFqn ?? "未确认"} · ID {m.externalId ?? "未确认"}</p>
        <DefinitionList items={[
          { label: "目录描述", value: g?.description || missing },
          { label: "目录 Owner", value: partyNames(g?.owners) || missing },
          { label: "目录 Steward", value: partyNames(g?.stewards) || missing },
          { label: "目录分类", value: g?.classifications?.join("、") || missing },
          { label: "目录术语", value: g?.terms?.join("、") || missing },
        ]} />
        <p>目录注释只读展示；上方 Core 归属和当前授权由平台管理。治理目录不授予认证或交付权限。</p>
        <CommandButton label="刷新治理绑定" action={refreshMetadata.bind(null, id, `dpp_source.${s.database}.${table?.Schema}.${table?.Name}`)} />
        <h3>同一事实链</h3>
        {linked.map(b => <p key={b.id}>
          <Link className="text-link" href={`/batches/${b.id}`}>{b.request.batchKey} · 查看批次与加工入口</Link>{" "}
          <Badge value={b.state} />
        </p>)}
      </section>
    );
  } catch {
    return <section className="callout callout-warn"><h2>来源信息 UNKNOWN</h2><p>来源/治理信息暂不可用，请刷新；Core 历史事实仍保留。</p></section>;
  }
}
