export const admissionText = {
  "zh-CN": {
    title: "接纳标注来源", description: "核验已批准的纯文本快照，保存为 RAW 数据集版本。认证和交付仍需独立完成。",
    release: "导出回执 ID", snapshot: "快照 ID", hash: "Manifest SHA-256", submit: "接纳 / 继续原请求", lookup: "查询原请求", fresh: "新建请求",
    busy: "正在核验…", accepted: "来源已接纳", prepared: "原请求尚未完成，可使用相同请求继续。", version: "数据集版本", count: "样本数", key: "请求标识",
    denied: "当前权限不足。已清除旧结果，请联系管理员核对权限。", integrity: "来源或留存副本完整性校验失败，不能接纳或自动修复。", conflict: "请求内容与已有记录冲突，请核对原请求。",
    unsupported: "此来源不符合首版纯文本 JSONL 契约。", unknown: "结果尚未确认。请查询原请求，不要另建请求来重试。", failed: "原请求已被拒绝；历史保留，请核对来源后使用新的快照。", setup: "接纳入口尚未配置或当前身份无权访问。",
  },
  "en-US": {
    title: "Accept annotation source", description: "Verify an approved text snapshot and retain it as a RAW dataset version. Certification and delivery remain separate.",
    release: "Export receipt ID", snapshot: "Snapshot ID", hash: "Manifest SHA-256", submit: "Accept / resume original request", lookup: "Check original request", fresh: "New request",
    busy: "Verifying…", accepted: "Source accepted", prepared: "The original request is incomplete. Resume using the same request.", version: "Dataset version", count: "Samples", key: "Request identity",
    denied: "Current access denied. Stale results were cleared; ask an administrator to check access.", integrity: "Source or retained-copy integrity failed. Acceptance and automatic repair are blocked.", conflict: "The input conflicts with an existing request. Check the original request.",
    unsupported: "This source does not meet the first-version text JSONL contract.", unknown: "The result is unknown. Check the original request instead of creating a replacement.", failed: "The original request was rejected. History is retained; check the source and use a new snapshot.", setup: "Source acceptance is not configured or this identity is not authorized.",
  },
} as const;
export type AdmissionLocale = keyof typeof admissionText;
