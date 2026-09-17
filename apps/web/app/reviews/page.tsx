import Link from "next/link";
import { Badge, LoadError, PageHeader, SetupRequired } from "@/components/ui";
import { ReviewQueue } from "@/components/review-queue";
import { configuredWorkspaceId, platform } from "@/lib/platform";
import { reviewConfigurationError } from "@/lib/review-command";

// POC workspace/reviewer settings are deployment-time server configuration.
// Do not freeze the SetupRequired/read-only branch during `next build`; the
// same production artifact must be usable with different trusted runtime envs.
export const dynamic = "force-dynamic";

export default async function ReviewsPage({ searchParams }: { searchParams: Promise<{ offset?: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Entity Resolution" title="实体审核" description="低置信度候选进入统一人工审核队列。" /><SetupRequired /></>;
  }
  const query = await searchParams;
  const requestedOffset = Number(query.offset ?? 0);
  const offset = Number.isSafeInteger(requestedOffset) && requestedOffset >= 0 ? Math.min(requestedOffset, 2147483647) : 0;
  const configurationError = reviewConfigurationError({
    enabled: process.env.POC_ENABLE_REVIEW_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_REVIEWER_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });
  try {
    const reviews = await platform.reviews("PENDING", 25, offset);
    return (
      <>
        <PageHeader eyebrow="Entity Resolution" title="实体审核"
          description="核对来源、规则和候选实体后，填写理由并确认或拒绝。任务状态、映射和证据由 Core 命令维护。"
          action={<Badge value={reviews.page.total ? "PENDING" : "NO_PENDING"} />} />
        {configurationError ? <div className="callout callout-warn"><strong>当前为只读审核队列</strong><p>{configurationError}</p></div> : <div className="callout callout-warn"><strong>POC 审核模式</strong><p>当前使用服务端配置的演示审核人；此配置不是登录认证或角色授权，请勿开放至不可信网络。</p></div>}
        <ReviewQueue items={reviews.items} actionsEnabled={!configurationError} />
        <nav aria-label="审核队列分页" className="badge-row" style={{ marginTop: 16 }}>
          {offset > 0 ? <Link href={`/reviews?offset=${Math.max(0, offset - 25)}`}>上一页</Link> : null}
          <span>本页 {reviews.items.length} 条 · 待审核共 {reviews.page.total} 条</span>
          {offset + reviews.items.length < reviews.page.total ? <Link href={`/reviews?offset=${offset + 25}`}>下一页</Link> : null}
        </nav>
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Entity Resolution" title="实体审核" /><LoadError error={error} /></>;
  }
}
