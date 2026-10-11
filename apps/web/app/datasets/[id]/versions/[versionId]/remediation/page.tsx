import Link from "next/link";
import { BackLink,Badge,LoadError,PageHeader } from "@/components/ui";
import { CommandButton } from "@/components/batch-controls";
import { RemediationForm } from "@/components/remediation-form";
import { batches,core,receipt,workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";
import { detectAnomaly,remediate } from "./actions";
type Case={id:string;version_id:string;assessment_id:string;finding_id:string;created_by:string;plan_ref:string;plan_version:string;plan_hash:string;workflow_hash:string;detection_rule_id:string};
type Detail={case:Case;finding:{message:string;observed:{affectedCount?:number;total?:number}};assignments:Array<{actor_id:string;reason:string}>;attempts:Array<{id:string;ordinal:number;executionId:string;reason:string;status:string;errorCode:string;errorMessage:string;outputDatasetId:string;outputVersionId?:string;metrics?:Record<string,number>}>;rechecks:Array<{id:string;requestKey:string;executionLinkId:string;outcome:string;errorCode?:string;assessmentId?:string;gateDecision?:string}>;closure:null|{reason:string}};
export default async function RemediationPage({params}:{params:Promise<{id:string;versionId:string}>}){
 const {id,versionId}=await params;const back=`/datasets/${id}/versions/${versionId}`;
 try{
  const [dataset,version,list,accepted]=await Promise.all([platform.dataset(id),platform.datasetVersion(versionId),core<{items:Case[]}>(`/workspaces/${workspace()}/quality-remediations?versionId=${versionId}`,"GET",undefined,undefined,true),batches()]);
  if(dataset.workspaceId!==workspace()||version.datasetId!==id)throw new Error("版本不属于当前 workspace");
  const choices:Array<{id:string;name:string}>=[];
  for(const b of accepted.filter(b=>b.state==="ACCEPTED")){if((await receipt(b.id)).bindings.some(v=>v.versionId===versionId))choices.push({id:b.id,name:b.request.batchKey});}
  const details=await Promise.all(list.items.map(c=>core<Detail>(`/workspaces/${workspace()}/quality-remediations/${c.id}`,"GET",undefined,undefined,true)));
  return <><BackLink href={back}>返回原始版本</BackLink><PageHeader title="用电异常处置" description="原始值保留。固定规则隔离负数，产出新版本后独立复检；认证与当前交付授权分别检查。"/>
   <section className="detail-card"><h2>发现异常</h2><p>来源：{dataset.name}；版本 <span className="mono">{versionId}</span>。只检测已接纳 RAW 的原值，未检测不表示合格。</p><CommandButton action={detectAnomaly.bind(null,id,versionId)} label="检测并登记负数用电异常" disabled={dataset.datasetType!=="RAW"}/></section>
   {details.map(d=>{const latest=d.attempts.at(-1);const assigned=d.assignments.at(-1);const quality=d.rechecks.at(-1);const closed=Boolean(d.closure);const recovering=d.rechecks.filter(q=>q.outcome==="PENDING");return <section className="detail-card" key={d.case.id}>
    <h2>异常 {d.case.id}</h2><p>{d.finding.message}；异常 {d.finding.observed.affectedCount??"未知"} / 已检测 {d.finding.observed.total??"未知"} 条用电记录。</p>
    <p>责任人：{assigned?.actor_id??"未分派"}。本试点明确分派给当前受信任操作者。</p><p>规则：{d.case.detection_rule_id}；处置方案 {d.case.plan_ref} @ {d.case.plan_version}；固定 workflow hash <span className="mono">{d.case.workflow_hash}</span>。</p>
    <p>状态：{closed?"人工关闭（不代表合格）":quality?(quality.gateDecision??(quality.outcome==="PENDING"?"复检待核对":quality.outcome==="FAILED"?"复检失败":quality.outcome)):latest?.status??(assigned?"已分派，待处理":"待分派")}。下游影响：仅展示已绑定执行与版本，其他下游未知。</p>
    {!assigned&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"assign")} label="分派给当前操作者"/>}
    {assigned&&!latest&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"execute")} label="按既有规则隔离并生成新版本" batches={choices} disabled={!choices.length}/>}
    {d.attempts.map(a=><article className="detail-card" key={a.id}><h3>处理 attempt {a.ordinal} <Badge value={a.status}/></h3><p>处理理由：{a.reason}。失败原因：{a.errorCode||"—"} {a.errorMessage||""}</p><Link className="text-link" href={`/production/${a.executionId}`}>查看实际执行与隔离原值</Link>
      {a.metrics&&<p>同粒度用电记录：输入 {a.metrics.energyRecords??"未知"}；纳入 {a.metrics.energyMapped??"未知"}；隔离 {a.metrics.quarantineCount??"未知"}。聚合输出为 {a.metrics.outputRows??"未知"} 企业行，单独计数。</p>}
      {a.outputVersionId&&<p><Link className="text-link" href={`/datasets/${a.outputDatasetId}/versions/${a.outputVersionId}`}>进入新版本质量、认证与交付</Link></p>}
    </article>)}
    {latest&&["FAILED","CANCELLED"].includes(latest.status)&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"retry")} label="明确失败后创建新执行重试"/>}
    {latest?.status==="SUCCEEDED"&&!closed&&!recovering.length&&<RemediationForm key={`new-recheck-${d.rechecks.length}`} action={remediate.bind(null,id,versionId,d.case.id,"recheck")} label={quality?"明确创建新的独立复检":"对新版本独立复检"} executionLinkId={latest.id}/>}
    {recovering.map(q=><div key={q.id}><p>复检结果待核对。恢复原请求会复用已接纳的评测，人工关闭也不会丢弃它；请先核对原结果。</p><RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"recheck")} label="恢复原复检" executionLinkId={q.executionLinkId} requestKey={q.requestKey}/></div>)}
    {d.rechecks.map(q=><p key={q.id}>复检 {q.id}：{q.outcome} {q.gateDecision??q.errorCode??"结果待核对"}；评测 {q.assessmentId??"尚未生成"}。通过也不自动认证或授予交付权。</p>)}
    {!closed&&assigned&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"close")} label="仅记录人工关闭"/>}
    {closed&&<p>人工关闭理由：{d.closure?.reason}。原始异常、旧评测和认证事实继续保留。</p>}
   </section>;})}
  </>;
 }catch(e){return <><BackLink href={back}>返回原始版本</BackLink><PageHeader title="用电异常处置"/><LoadError error={e}/></>;}
}
