import {displayText} from '../../lib/business-language.mjs';
import Link from "next/link";
import { Badge,LoadError,PageHeader,SetupRequired } from "@/components/ui";
import { RegisterBatchForm } from "@/components/batch-controls";
import { batchEnabled,batches,source } from "@/lib/batches";
export default async function BatchesPage(){
  if(!batchEnabled())return <><PageHeader title="数据库批次"/><SetupRequired/></>;
  try{const [s,items]=await Promise.all([source(),batches()]);return <><PageHeader title="数据库批次" description="来源 → 三份 原始数据 原子接纳 → 实体审核 → 加工 → 质量、认证和当前授权交付。"/><section className="detail-card"><h2>受控来源</h2><p>{displayText(s.database)} · {displayText(s.connectionRef)} · {displayText(s.instanceRef)} · 只读账户 {displayText(s.reader)}</p><p>{displayText(s.tables.map(t=>`${t.Schema}.${t.Name}（${t.Rows} 行）`).join(" / "))}</p></section><RegisterBatchForm/><section className="table-card"><table className="data-table"><thead><tr><th>批次</th><th>状态</th><th>评测记录</th></tr></thead><tbody>{items.map(b=><tr key={b.id}><td><Link href={`/batches/${b.id}`} className="text-link">{displayText(b.request.batchKey)}</Link></td><td><Badge value={b.state}/></td><td className="mono">{displayText(b.attemptId)}</td></tr>)}</tbody></table><p>显示最近 100 个本人批次；无法确认 必须核对原运行，只有明确 失败 才能创建新 评测记录。</p></section></>;}catch(e){return <><PageHeader title="数据库批次"/><LoadError error={e}/></>;}
}
