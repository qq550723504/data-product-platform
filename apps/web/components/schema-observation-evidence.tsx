import {businessText,displayText} from '../lib/business-language.mjs';
import Link from "next/link";
import { Badge, DefinitionList, LoadError, PageHeader, formatDate } from "./ui";
import { schemaObservation } from "@/lib/schema-observation-client";
import { SchemaColumnComparison } from "./source-schema-comparison";
import { attentionOffset } from "@/lib/quality-attention";
import { schemaExceptionsHref } from "@/lib/schema-exceptions";
import { isSchemaExceptionStatus, schemaExceptionFilter } from "@/lib/schema-exception-filter";
import { schemaRequestHref, schemaRequestOrigin } from "@/lib/schema-requests";
import { schemaRequestDetail } from "@/lib/schema-request-client";

export async function SchemaObservationEvidence({ resourceId, requestId, observationId, exceptionStatus, exceptionOffset, requestReturn }: { resourceId: string; requestId?: string; observationId?: string; exceptionStatus?: string; exceptionOffset?: string; requestReturn?: Record<string, string | undefined> }) {
  try {
    const o = await schemaObservation(resourceId, requestId ?? "", observationId ?? "");
    const origin = requestReturn ? schemaRequestOrigin(requestReturn) : undefined;
    const selected = requestReturn?.requestAttemptId;
    // A selected return attempt may differ from this observation's attempt, but
    // it must still belong to the exact request/resource/workspace. Never reselect.
    if (selected !== undefined) await schemaRequestDetail(resourceId, o.requestId, selected, 0, 0);
    const exception = isSchemaExceptionStatus(o.status);
    const back = requestReturn && origin ? schemaRequestHref(resourceId, o.requestId, origin, selected, attentionOffset(requestReturn.attemptOffset), attentionOffset(requestReturn.observationOffset)) : exception ? schemaExceptionsHref({}, schemaExceptionFilter(exceptionStatus), attentionOffset(exceptionOffset)) : "/attention?schemaView=ALL#source-schema-attention";
    return <>
      <Link prefetch={false} className="text-link" href={back}>{businessText(requestReturn ? "返回精确请求证据" : exception ? "返回扫描异常历史" : "返回漂移观察")}</Link>
      <PageHeader title="来源 数据结构 历史观察" eyebrow={o.resourceCode} description={`${o.resourceName} · 精确历史证据，只读`} action={<Badge value={o.status} />} />
      <section className="detail-card" data-testid="schema-observation-evidence">
        <h2>观察 {displayText(o.observationId)}</h2>
        <p>当前仅展示指定扫描请求的历史观察，不读取当前 metadata 或扫描来源。{businessText(exception ? "历史异常不表示当前未恢复；" : "已记录处理不等于恢复；")}历史观察不证明当前来源结构，也不是冻结数据版本的质量、认证或交付结论。</p>
        <DefinitionList items={[
          { label: "Workspace", value: o.workspaceId }, { label: "资源 ID", value: o.resourceId },
          { label: "扫描请求", value: o.requestId }, { label: "实际 attempt", value: o.attemptId ?? "未绑定 attempt" },
          { label: "扫描责任人", value: o.responsibleActorId }, { label: "历史来源", value: o.externalFqn },
          { label: "历史 binding", value: o.bindingId }, { label: "历史外部 ID", value: o.externalId },
          { label: "规则", value: `${o.ruleRef} / ${o.ruleVersion}` }, { label: "规则 hash", value: o.ruleHash },
          { label: "观察契约 hash", value: o.planHash }, { label: "Connector hash", value: o.connectorHash },
          { label: "schema hash", value: o.schemaHash || "UNKNOWN" }, { label: "原因", value: o.reason },
          { label: "请求时间", value: formatDate(o.requestedAt) }, { label: "扫描开始", value: o.startedAt ? formatDate(o.startedAt) : "UNKNOWN" },
          { label: "扫描完成", value: o.completedAt ? formatDate(o.completedAt) : "UNKNOWN" }, { label: "记录时间", value: formatDate(o.recordedAt) },
        ]} />
        <h3>固定基线与本次扫描</h3>
        <SchemaColumnComparison baseline={o.baseline} columns={o.columns} conclusive={o.status === "DRIFT" || o.status === "MATCH"} />
        {exception ? <p>此记录属于扫描异常历史，不计为当前未恢复待办；后续成功仍保留旧异常。扫描异常不使用漂移的处理登记；当次拒绝也不代表资源永久不可用。</p> : <>
          <h3>处理记录</h3>
          {o.handling.length ? o.handling.map(h => <p key={h.id}>{displayText(h.reason)} · {displayText(h.actorId)} · {displayText(formatDate(h.createdAt))}</p>) : <p>本观察未记录处理。</p>}
        </>}
        <p>未知下游影响：无法确认。此页面不发起扫描、处理或恢复确认。</p>
      </section>
    </>;
  } catch (error) {
    return <><PageHeader title="来源 数据结构 历史观察" /><section data-testid="schema-observation-error"><LoadError error={error} /></section></>;
  }
}
