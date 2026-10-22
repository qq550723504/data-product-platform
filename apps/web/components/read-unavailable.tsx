export function ReadUnavailable({ title }: { title: string }) {
  return <div className="callout callout-warn" role="status" data-testid="read-unavailable">
    <strong>{title}：UNKNOWN</strong>
    <p>读取未完成、失败、权限不足或响应无效，本次无法确认。其他分区按各自读取结果展示。</p>
  </div>;
}
