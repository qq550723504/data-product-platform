import {businessText,displayText} from '../../../../../../lib/business-language.mjs';
import Link from "next/link";
import { assessmentHref, assertCaseScope, isIdentity } from "@/lib/quality-attention";
import { BackLink,Badge,LoadError,PageHeader } from "@/components/ui";
import { CommandButton } from "@/components/batch-controls";
import { RemediationForm } from "@/components/remediation-form";
import { batches,core,receipt,workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";
import { detectAnomaly,remediate } from "./actions";
type Case={id:string;workspace_id:string;version_id:string;assessment_id:string;finding_id:string;created_by:string;plan_ref:string;plan_version:string;plan_hash:string;workflow_hash:string;detection_rule_id:string};
type Detail={case:Case;finding:{message:string;observed:{affectedCount?:number;total?:number}};assignments:Array<{actor_id:string;reason:string}>;attempts:Array<{id:string;ordinal:number;executionId:string;reason:string;status:string;errorCode:string;errorMessage:string;outputDatasetId:string;outputVersionId?:string;metrics?:Record<string,number>}>;rechecks:Array<{id:string;requestKey:string;executionLinkId:string;outcome:string;errorCode?:string;assessmentId?:string;gateDecision?:string}>;closure:null|{reason:string}};
export default async function RemediationPage({params,searchParams}:{params:Promise<{id:string;versionId:string}>;searchParams:Promise<{caseId?:string}>}){
 const {id,versionId}=await params;const back=`/datasets/${id}/versions/${versionId}`;
 try{
  const focused=(await searchParams).caseId;
  if(focused&&!isIdentity(focused))throw new Error("异常引用无效");
  const [dataset,version,list,accepted]=await Promise.all([platform.dataset(id),platform.datasetVersion(versionId),focused?Promise.resolve({items:[] as Case[]}):core<{items:Case[]}>(`/workspaces/${workspace()}/quality-remediations?versionId=${versionId}`,"GET",undefined,undefined,"human-decision"),focused?Promise.resolve([]):batches()]);
  if(dataset.workspaceId!==workspace()||version.datasetId!==id)throw new Error("版本不属于当前 workspace");
  const choices:Array<{id:string;name:string}>=[];
  for(const b of accepted.filter(b=>b.state==="ACCEPTED")){if((await receipt(b.id)).bindings.some(v=>v.versionId===versionId))choices.push({id:b.id,name:b.request.batchKey});}
  const details=focused?[assertCaseScope(await core<Detail>(`/workspaces/${workspace()}/quality-remediations/${focused}`,"GET",undefined,undefined,"human-decision"),focused,versionId,workspace())]:await Promise.all(list.items.map(async c=>assertCaseScope(await core<Detail>(`/workspaces/${workspace()}/quality-remediations/${c.id}`,"GET",undefined,undefined,"human-decision"),c.id,versionId,workspace())));
  return <><BackLink href={back}>返回原始版本</BackLink><PageHeader title="用电异常处置" description="原始值保留。固定规则隔离负数，产出新版本后独立复检；认证与当前交付授权分别检查。"/>
   {focused?<p>当前为单个异常的只读导航。<Link href={back+"/remediation"}>进入版本原有处置入口</Link></p>:<section className="detail-card"><h2>发现异常</h2><p>来源：{businessText(dataset.name)}；版本 <span className="mono">{displayText(versionId)}</span>。只检测已接纳 原始数据 的原值，未检测不表示合格。</p><CommandButton action={detectAnomaly.bind(null,id,versionId)} label="检测并登记负数用电异常" disabled={dataset.datasetType!=="RAW"}/></section>}
   {details.map(d=>{const latest=d.attempts.at(-1);const assigned=d.assignments.at(-1);const quality=d.rechecks.at(-1);const closed=Boolean(d.closure);const recovering=d.rechecks.filter(q=>q.outcome==="PENDING");return <section className="detail-card" key={d.case.id}>
    <h2>异常 {businessText(d.case.id)}</h2><p>{businessText(d.finding.message)}；异常 {displayText(d.finding.observed.affectedCount??"未知")} / 已检测 {displayText(d.finding.observed.total??"未知")} 条用电记录。</p>
    <p>责任人：{displayText(assigned?.actor_id??"未分派")}。本试点明确分派给当前受信任操作者。</p><p>规则：{displayText(d.case.detection_rule_id)}；处置方案 {displayText(d.case.plan_ref)} @ {displayText(d.case.plan_version)}；固定 workflow hash <span className="mono">{displayText(d.case.workflow_hash)}</span>。</p>
    <p>执行：{businessText(latest?.status??"未执行")}；最新复检调用：{displayText(quality?.outcome??"尚未复检")}；复检质量结论：{businessText(quality?.gateDecision??"未知 / 尚无结论")}；人工关闭：{businessText(closed?"已关闭（不代表合格）":"未关闭")}。下游影响：仅展示已绑定执行与版本，其他下游未知。</p>
    <p><Link href={assessmentHref(id,versionId,d.case.assessment_id)}>查看原评测、Finding 与证据</Link> · Finding <span className="mono">{displayText(d.case.finding_id)}</span></p>
    {displayText(!focused&&!assigned&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"assign")} label="分派给当前操作者"/>)}
    {displayText(!focused&&assigned&&!latest&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"execute")} label="按既有规则隔离并生成新版本" batches={choices} disabled={!choices.length}/>)}
    {d.attempts.map(a=><article className="detail-card" key={a.id}><h3>处理 评测记录 {displayText(a.ordinal)} <Badge value={a.status}/></h3><p>处理理由：{displayText(a.reason)}。失败原因：{displayText(a.errorCode||"—")} {displayText(a.errorMessage||"")}</p><Link className="text-link" href={`/production/${a.executionId}`}>查看实际执行与隔离原值</Link>
      {a.metrics&&<p>同粒度用电记录：输入 {displayText(a.metrics.energyRecords??"未知")}；纳入 {displayText(a.metrics.energyMapped??"未知")}；隔离 {displayText(a.metrics.quarantineCount??"未知")}。聚合输出为 {displayText(a.metrics.outputRows??"未知")} 企业行，单独计数。</p>}
      {displayText(a.outputVersionId&&<p><Link className="text-link" href={`/datasets/${a.outputDatasetId}/versions/${a.outputVersionId}`}>进入新版本质量、认证与交付</Link></p>)}
    </article>)}
    {displayText(!focused&&latest&&["FAILED","CANCELLED"].includes(latest.status)&&!closed&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"retry")} label="明确失败后创建新执行重试"/>)}
    {displayText(!focused&&latest?.status==="SUCCEEDED"&&!closed&&!recovering.length&&<RemediationForm key={`new-recheck-${d.rechecks.length}`} action={remediate.bind(null,id,versionId,d.case.id,"recheck")} label={quality?"明确创建新的独立复检":"对新版本独立复检"} executionLinkId={latest.id}/>)}
    {recovering.map(q=><div key={q.id}><p>复检结果待核对。恢复原请求会复用已接纳的评测，人工关闭也不会丢弃它；请先核对原结果。</p>{displayText(!focused&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"recheck")} label="恢复原复检" executionLinkId={q.executionLinkId} requestKey={q.requestKey}/>)}</div>)}
    {d.rechecks.map(q=>{const output=d.attempts.find(a=>a.id===q.executionLinkId);return <p key={q.id}>复检 {businessText(q.id)}：{displayText(q.outcome)} {displayText(q.gateDecision??q.errorCode??"结果待核对")}；{q.assessmentId&&output?.outputVersionId?<Link href={assessmentHref(output.outputDatasetId,output.outputVersionId,q.assessmentId)}>查看该次复检报告与证据</Link>:<>评测 {displayText(q.assessmentId??"尚未生成")}</>}。通过也不自动认证或授予交付权。</p>;})}
    {displayText(!focused&&!closed&&assigned&&<RemediationForm action={remediate.bind(null,id,versionId,d.case.id,"close")} label="仅记录人工关闭"/>)}
    {closed&&<p>人工关闭理由：{displayText(d.closure?.reason)}。原始异常、旧评测和认证事实继续保留。</p>}
   </section>;})}
  </>;
 }catch(e){return <><BackLink href={back}>返回原始版本</BackLink><PageHeader title="用电异常处置"/><LoadError error={e}/></>;}
}
