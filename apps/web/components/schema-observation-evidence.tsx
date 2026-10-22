import Link from "next/link";
import { Badge, DefinitionList, LoadError, PageHeader, formatDate } from "./ui";
import { schemaObservation } from "@/lib/schema-observation-client";
import { SchemaColumnComparison } from "./source-schema-comparison";

export async function SchemaObservationEvidence({ resourceId, requestId, observationId }: { resourceId: string; requestId?: string; observationId?: string }) {
  try {
    const o = await schemaObservation(resourceId, requestId ?? "", observationId ?? "");
    return <>
      <Link prefetch={false} className="text-link" href="/attention?schemaView=ALL#source-schema-attention">返回漂移观察</Link>
      <PageHeader title="来源 schema 历史观察" eyebrow={o.resourceCode} description={`${o.resourceName} · 精确历史证据，只读`} action={<Badge value={o.status} />} />
      <section className="detail-card" data-testid="schema-observation-evidence">
        <h2>观察 {o.observationId}</h2>
        <p>当前仅展示指定扫描请求的历史观察，不读取当前 metadata 或扫描来源。已记录处理不等于恢复；历史观察不证明当前来源结构，也不是冻结数据版本的质量、认证或交付结论。</p>
        <DefinitionList items={[
          { label: "Workspace", value: o.workspaceId }, { label: "资源 ID", value: o.resourceId },
          { label: "扫描请求", value: o.requestId }, { label: "实际 attempt", value: o.attemptId ?? "未启动" },
          { label: "扫描责任人", value: o.responsibleActorId }, { label: "历史来源", value: o.externalFqn },
          { label: "历史 binding", value: o.bindingId }, { label: "历史外部 ID", value: o.externalId },
          { label: "规则", value: `${o.ruleRef} / ${o.ruleVersion}` }, { label: "规则 hash", value: o.ruleHash },
          { label: "观察契约 hash", value: o.planHash }, { label: "Connector hash", value: o.connectorHash },
          { label: "schema hash", value: o.schemaHash || "UNKNOWN" }, { label: "原因", value: o.reason },
          { label: "请求时间", value: formatDate(o.requestedAt) }, { label: "扫描开始", value: formatDate(o.startedAt) },
          { label: "扫描完成", value: formatDate(o.completedAt) }, { label: "记录时间", value: formatDate(o.recordedAt) },
        ]} />
        <h3>固定基线与本次扫描</h3>
        <SchemaColumnComparison baseline={o.baseline} columns={o.columns} conclusive={o.status === "DRIFT" || o.status === "MATCH"} />
        <h3>处理记录</h3>
        {o.handling.length ? o.handling.map(h => <p key={h.id}>{h.reason} · {h.actorId} · {formatDate(h.createdAt)}</p>) : <p>本观察未记录处理。</p>}
        <p>未知下游影响：UNKNOWN。此页面不发起扫描、处理或恢复确认。</p>
      </section>
    </>;
  } catch (error) {
    return <><PageHeader title="来源 schema 历史观察" /><section data-testid="schema-observation-error"><LoadError error={error} /></section></>;
  }
}
