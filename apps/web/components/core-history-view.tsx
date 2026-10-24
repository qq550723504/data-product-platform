import {UnknownHistory,CertificationRecords,ReleaseRecords} from "./core-history-records";
import {PageHeader,EmptyState} from '@/components/ui';
import {configuredWorkspaceId,type CertificationHistory} from '@/lib/platform';
import {qualityRead} from '@/lib/quality-server';
import {readSection} from '@/lib/workbench-read';
import {readAgentContext} from '@/lib/agent-quality-server';
import {agentSelection,validAgentContext} from '@/lib/agent-quality';
import {HISTORY_LIMIT,RELEASE_STATUSES,historySelection,historyHref,validCertificationHistory,validReleaseHistory,type ReleaseHistory} from '@/lib/core-history';

export default async function CoreHistoryView({searchParams,certification=false}:{searchParams:Promise<Record<string,string|string[]|undefined>>;certification?:boolean}){
 const base=certification?'/core/certifications':'/core/releases';const ws=configuredWorkspaceId()??'';
 let s;try{s=historySelection(await searchParams,certification);}catch{return <><PageHeader title="历史查询引用无效"/><UnknownHistory>请选择明确版本，使用有效的分页与历史时点。</UnknownHistory><a href="/core/assets">返回资产目录</a></>;}
 const nav=<nav className="capability-links" aria-label="Core 只读导航"><a href="/core/assets">资产目录</a><a href="/core/releases">发布历史</a><a href="/core/release-trace">发布证据链</a><a href="/core/deliveries">交付记录</a><a href="/core/archives">证据档案</a>{certification&&<a href={historyHref('/core/assets',{datasetId:s.datasetId,versionId:s.versionId})}>返回版本与质量</a>}</nav>;
 const title=certification?'认证历史':'发布历史';
 const header=<>{nav}<PageHeader title={title} description="只读服务端历史事实；不运行认证、发布或交付。"/><p>工作空间：<code>{ws||'未配置'}</code></p></>;
 if(certification){
  const source=agentSelection({datasetId:s.datasetId,versionId:s.versionId});const ctx=await readSection(()=>readAgentContext(source),c=>validAgentContext(c,ws,source));
  if(!ctx?.version)return <>{header}<UnknownHistory>所选版本或工作空间不可确认，没有读取其他版本的认证。</UnknownHistory></>;
  const q=new URLSearchParams({limit:String(HISTORY_LIMIT),offset:String(s.offset)});if(s.asOf)q.set('asOf',s.asOf);if(s.anchorRevision!==undefined)q.set('anchorRevision',String(s.anchorRevision));
  const h=await readSection(()=>qualityRead<CertificationHistory>(`/api/v1/workspaces/${ws}/dataset-versions/${s.versionId}/certifications?${q}`),h=>validCertificationHistory(h,ws,s));
  if(!h)return <>{header}<p>明确版本：{ctx.version.id}</p><UnknownHistory>认证历史读取失败，当前状态无法确认。质量报告仍可从版本页独立查看。</UnknownHistory></>;
  const anchored={...s,asOf:h.asOf,anchorRevision:h.page.anchorRevision};
  return <>{header}<section className="card"><h2>{ctx.dataset?.name} · v{ctx.version.versionNo}</h2><p>明确版本：{ctx.version.id}</p><p>查询时点：{h.asOf} · 历史修订：{h.page.anchorRevision}</p><a className="text-link" href={historyHref(base,{datasetId:s.datasetId,versionId:s.versionId})}>刷新到当前记录</a><p>历史 CERTIFIED 不代表当前交付许可。认证时结论与后续撤销、替代记录分别展示。</p>{!h.page.total?<EmptyState title="暂无认证记录" description="此版本尚无认证历史，不能据此判断已认证。"/>:null}{(h.page.total>0||s.certificationId)&&<CertificationRecords history={h} selected={s.certificationId}/>}<HistoryPages base={base} selection={anchored} total={h.page.total}/></section></>;
 }
 const q=new URLSearchParams({limit:String(HISTORY_LIMIT),offset:String(s.offset)});if(s.status)q.set('status',s.status);
 const h=await readSection(()=>qualityRead<ReleaseHistory>(`/api/v1/workspaces/${ws}/product-releases?${q}`),h=>validReleaseHistory(h,ws,s));
 return <>{header}<form method="get" action={base} className="agent-lookup"><label htmlFor="release-status">发布状态</label><select name="status" id="release-status" defaultValue={s.status}><option value="">全部状态</option>{RELEASE_STATUSES.map(x=><option key={x}>{x}</option>)}</select><button className="button">筛选历史</button></form><p>此处查询整个授权工作空间的发布批次；不推断其与某个所选资产的成员关系。保留状态枚举不表示该状态迁移已实现。</p>{!h?<UnknownHistory>发布历史读取失败，没有使用演示批次替代。</UnknownHistory>:<section className="card">{!h.page.total?<EmptyState title="暂无发布记录" description="当前筛选未返回发布批次，不表示具备发布或交付资格。"/>:null}{(h.page.total>0||s.releaseId)&&<ReleaseRecords history={h} selected={s.releaseId} offset={s.offset} status={s.status}/>}<HistoryPages base={base} selection={s} total={h.page.total}/></section>}</>;
}
function HistoryPages({base,selection:s,total}:{base:string;selection:ReturnType<typeof historySelection>;total:number}){return <nav className="pagination" aria-label="历史分页">{s.offset>0?<a href={historyHref(base,{...s,offset:Math.max(0,s.offset-HISTORY_LIMIT)})}>上一页</a>:<span>已到首页</span>}<span>共 {total} 条 · 偏移 {s.offset}</span>{s.offset+HISTORY_LIMIT<total?<a href={historyHref(base,{...s,offset:s.offset+HISTORY_LIMIT})}>下一页</a>:<span>已到末页</span>}</nav>;}
