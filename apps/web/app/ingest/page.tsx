import { randomUUID } from "node:crypto";
import { PageHeader, SetupRequired } from "@/components/ui";
import { CSVIngest } from "@/components/csv-ingest";
import { configuredWorkspaceId } from "@/lib/platform";
import { ingestConfigurationError } from "@/lib/ingest-command";
export default async function IngestPage({ searchParams }: { searchParams: Promise<{ version?: string }> }) {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) return <><PageHeader title="数据接入" /><SetupRequired /></>;
  const { version } = await searchParams;
  const resumeVersionId = typeof version === "string" && /^[0-9a-f-]{36}$/i.test(version) ? version : undefined;
  const configurationError = ingestConfigurationError({ enabled: process.env.POC_ENABLE_INGEST_ACTIONS === "true", workspaceId, actorId: process.env.POC_INGEST_ACTOR_ID?.trim(), apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080" });
  return <>
    <PageHeader eyebrow="Data Ingestion · V1" title="数据接入" description="从自己的 CSV 创建 RAW 版本，显式启动主体解析，再进入已有人工审核流程。" />
    <div className="callout callout-warn" style={{ marginBottom: 20 }}>
      <strong>{configurationError ? "当前为只读接入预览" : "受信任 POC 接入"}</strong>
      <p>{configurationError ?? "操作人由服务端配置，不等于真实登录认证。仅使用合成或获准在此环境处理的数据，不要开放到公网。"}</p>
      <p>本阶段使用主体基础信息模板；来源声明不等于授权审批，CSV 预检不等于质量、合规或发布检查通过。</p>
    </div>
    <CSVIngest operationId={randomUUID()} enabled={!configurationError} resumeVersionId={resumeVersionId} />
  </>;
}
