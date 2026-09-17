import { EmptyState, PageHeader } from "@/components/ui";

export default function EvidencePage() {
  return (
    <>
      <PageHeader
        eyebrow="Evidence"
        title="证据中心"
        description="证据不是附件文件夹，而是对权利、来源、加工、质量、合规、成本与发布事实的可追溯证明。"
      />
      <section className="detail-card" style={{ marginBottom: 18 }}>
        <h2>POC 证据链目标</h2>
        <div className="flow" style={{ marginTop: 16 }}>
          {[
            "ProductRelease",
            "DatasetVersion",
            "Execution",
            "Input DatasetVersion",
            "Data Resource",
            "Evidence",
          ].map((step, index, steps) => (
            <span key={step} className="flow">
              <span className="flow-step">{step}</span>
              {index < steps.length - 1 ? <span className="flow-arrow">→</span> : null}
            </span>
          ))}
        </div>
      </section>
      <EmptyState
        title="证据图浏览将在下一阶段接入"
        description="当前入口先固定产品信息架构；后续直接读取 Core Evidence/Traceability，而不会把外部治理或执行引擎当成证据系统。"
      />
    </>
  );
}
