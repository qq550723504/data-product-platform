import {qualityRead} from '@/lib/quality-server';
import {qualityCosts} from '@/lib/quality-cost';
import {UnknownHistory} from './core-history-records';
import {EmptyState} from './ui';
export default async function CoreQualityCost({workspace,version,assessment}:{workspace:string;version:string;assessment:string}){
 let costs;try{costs=qualityCosts(await qualityRead<unknown>(`/api/v1/quality-assessments/${assessment}?workspaceId=${encodeURIComponent(workspace)}`),workspace,version,assessment);}catch{costs=null;}
 return <section className="card" id="quality-cost"><h2>本次评测成本</h2><p>评测：{assessment}</p><p>按服务端成本分配关系读取。未定价不等于免费，不合计不同币种；此处不代表结算账单。</p>{costs===null?<UnknownHistory>成本记录不可确认，已有评测报告和证据仍独立展示。</UnknownHistory>:!costs.length?<EmptyState title="未返回关联成本记录" description="不据此判断本次活动没有成本。"/>:<div className="table-wrap"><table><thead><tr><th>成本 / 实际活动</th><th>类型 / 用量</th><th>金额 / 定价</th><th>发生时间</th></tr></thead><tbody>{costs.map(c=><tr key={c.id}><td>{c.id}<p>活动：{c.activityId||'未返回'}</p><p>执行：{c.executionId||'无关联执行'}</p></td><td>{c.costType}<p>{c.quantity} {c.unit}</p></td><td>{c.amount===undefined?'金额尚未确定':`${c.amount} ${c.currency||'币种未返回'}`}<p>{c.pricingMode}</p></td><td>{c.occurredAt}</td></tr>)}</tbody></table></div>}</section>;
}
