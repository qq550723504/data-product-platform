import {core,workspace} from "@/lib/batches";
import {Badge,formatDate} from "./ui";
import {SchemaScanForm,SchemaHandlingForm} from "./schema-controls";

type Column={name:string;type:string;constraint:string};
type Observation={id:string;status:string;reason:string;columns:Column[];schema_hash:string;started_at?:string;completed_at?:string;recorded_at:string};
type Request={id:string;responsibleActorId:string;ruleRef:string;ruleVersion:string;ruleHash:string;planHash:string;externalFqn:string;baseline:Column[];requestedAt:string;observations:Observation[];handling:Array<{observation_id:string;reason:string;created_at:string}>};
export async function SourceSchema({resourceId,providerAvailable}:{resourceId:string;providerAvailable:boolean}){
 try{
  const history=await core<{enabled:boolean;items:Request[]}>(`/data-resources/${resourceId}/schema-observations?workspaceId=${workspace()}`,"GET",undefined,undefined,true);
  if(!history.enabled&&!history.items.length)return null;
  return <section className="detail-card"><h2>来源 schema 持续治理</h2>
   <p>观察范围：固定用电表；由原生 PostgreSQL Connector 扫描。最近扫描结论不是实时来源保证，也不是某个冻结数据版本的质量证明。</p>
   {!providerAvailable&&<p role="alert">UNKNOWN：metadata 当前失联。下列扫描仅为历史证据，不能确认当前来源结构；重连后须请求新的原生扫描。</p>}
   <p>schema 变化须在来源侧按批准契约处理；平台不会自动接受新列、修改旧 RAW 或认证。未知下游影响：UNKNOWN。</p>
   {history.enabled&&<SchemaScanForm resourceId={resourceId}/>}
   {history.items.length===0&&<p>NOT_RUN：尚无原生来源扫描事实。</p>}
   {history.items.map((r,i)=>{const o=r.observations.at(-1);const handled=o&&r.handling.some(h=>h.observation_id===o.id);return <article key={r.id}>
    <h3>{i===0?"最近扫描请求":"历史扫描请求"} · {r.id}</h3><Badge value={o?.status??"REQUESTED"}/>
    <p>责任人 {r.responsibleActorId} · {o?.status==="DRIFT"?(handled?"处理中":"待处理"):o?.status==="MATCH"?"本次结构与基线一致":"尚未确认恢复"}</p>
    <p>来源 {r.externalFqn} · 规则 {r.ruleRef} / {r.ruleVersion} · 规则 hash {r.ruleHash} · 观察契约 hash {r.planHash}</p>
    <p>请求 {formatDate(r.requestedAt)} · 原生扫描完成 {formatDate(o?.completed_at)} · 记录 {formatDate(o?.recorded_at)}</p>
    <p>原因 {o?.reason??"等待现有 worker 执行"} · schema hash {o?.schema_hash||"UNKNOWN"}</p>
    <table><thead><tr><th>列</th><th>固定基线</th><th>本次扫描</th></tr></thead><tbody>{Array.from(new Set([...r.baseline.map(c=>c.name),...(o?.columns??[]).map(c=>c.name)])).map(name=>{const b=r.baseline.find(c=>c.name===name),c=o?.columns.find(c=>c.name===name);return <tr key={name}><td>{name}</td><td>{b?`${b.type} / ${b.constraint}`:"基线无此列"}</td><td>{c?`${c.type} / ${c.constraint}`:o?.status==="MATCH"||o?.status==="DRIFT"?"本次无此列":"UNKNOWN"}</td></tr>;})}</tbody></table>
    {o?.status==="DRIFT"&&!handled&&history.enabled&&<SchemaHandlingForm resourceId={resourceId} observationId={o.id}/>}
    {r.handling.map(h=><p key={h.observation_id+h.created_at}>处理记录：{h.reason} · {formatDate(h.created_at)}</p>)}
    {r.observations.length>1&&<p>本请求保留全部观察：{r.observations.map(x=>`${x.status} (${formatDate(x.recorded_at)})`).join(" → ")}</p>}
   </article>;})}
  </section>;
 }catch{return <section className="detail-card"><h2>来源 schema 持续治理</h2><p role="alert">UNKNOWN：来源扫描历史当前不可读取，不能确认恢复。</p></section>;}
}
