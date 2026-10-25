import {businessText} from '../lib/business-language.mjs';
export function ReadUnavailable({ title }: { title: string }) {
  return <div className="callout callout-warn" role="status" data-testid="read-unavailable">
    <strong>{businessText(title)}：无法确认</strong>
    <details><summary>查看原因</summary><p>读取尚未完成或当前不可用，请核对访问权限后重新查询。其他分区仍保留各自的读取结果。</p></details>
  </div>;
}
