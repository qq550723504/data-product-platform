// Test-only, in-memory source. Never imported by application code or configured in a preview.
import {randomBytes,createHmac,createHash} from 'node:crypto';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {LogReadError,logNow}=require('../../.runtime-log-tests/lib/runtime-logs.js');
const opaque=()=>randomBytes(32).toString('base64url');
export function syntheticSource(){
 const key=randomBytes(32),references=new Map();
 const state={version:1,source:'CONFIGURED',allow:true,logRead:true,records:[],calls:[],afterRead:null,fault:null};
 const base=logNow(Date.now()-60000).slice(0,20)+'123456789Z',end=logNow(),start=logNow(Date.now()-3600000);
 const ns=BigInt(base.slice(20,29));
 for(const workspace of ['A','B'])for(let i=0;i<58;i++)state.records.push({id:String(i).padStart(3,'0'),workspace,recordedAt:i<28?base:base.slice(0,20)+(ns>0n?ns-1n:0n).toString().padStart(9,'0')+'Z',occurredAt:i===57?null:base,service:i%2?'APP_SERVICE':'BACKGROUND_SERVICE',level:i%3?'INFO':'WARN',eventCode:i%5?'READ_COMPLETED':'READ_FAILED'});
 const window={start,end};
 const rowKey=(r)=>createHmac('sha256',key).update(r.workspace+':'+state.version+':'+r.id).digest('base64url');
 const hash=r=>createHash('sha256').update(JSON.stringify([r.recordedAt,r.occurredAt,r.service,r.level,r.eventCode])).digest('hex');
 function reader(confirm,workspace){
  const checked=async(records=true)=>{const p=await confirm();if(!p.capabilities.includes('PLATFORM_MANAGEMENT_READ')||records&&(!state.allow||!state.logRead))throw new LogReadError('LOG_ACCESS_DENIED');return p};
  const wrap=(p,data)=>({profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:new Date().toISOString(),source:'SYNTHETIC_FIXTURE',writeEnabled:false,data});
  function ref(p,claims){const value=opaque();references.set(value,{...claims,workspace,principal:p.principal_id,revision:p.revision,version:state.version,expires:Math.min(Date.now()+900000,Date.parse(p.expires_at))});return value}
  function recover(value,p,kind){const r=references.get(value);if(!r)throw new LogReadError('INVALID_LOG_QUERY');if(r.workspace!==workspace||r.principal!==p.principal_id)throw new LogReadError('LOG_ACCESS_DENIED');if(r.revision!==p.revision||r.version!==state.version||r.expires<=Date.now()||r.kind!==kind)throw new LogReadError('LOG_QUERY_EXPIRED');return r}
  function project(p,r,s,asOf){return {rowKey:rowKey(r),entryRef:ref(p,{kind:'entry',id:r.id,hash:hash(r),s,asOf}),recordedAt:r.recordedAt,occurredAt:r.occurredAt,service:r.service,level:r.level,eventCode:r.eventCode}}
  return {
   async confirm(){const p=await confirm();return {...p,capabilities:[...p.capabilities,...state.logRead&&state.allow?['PLATFORM_RUNTIME_LOG_READ']:[]]}},
   async source(){const p=await checked(false);return wrap(p,{state:state.source,availability:'NOT_CHECKED'})},
   async query(s){const p=await checked(),version=state.version;state.calls.push({workspace,kind:'query'});let asOf=logNow(),after;
    if(s.cursor){const c=recover(s.cursor,p,'cursor');for(const k of ['start','end','service','level'])if(c.s[k]!==s[k])throw new LogReadError('INVALID_LOG_QUERY');asOf=c.asOf;after=c.after}
    const records=state.records.filter(r=>r.workspace===workspace&&r.recordedAt>=s.start&&r.recordedAt<s.end&&r.recordedAt<=asOf&&(!s.service||r.service===s.service)&&(!s.level||r.level===s.level)).sort((a,b)=>b.recordedAt.localeCompare(a.recordedAt)||b.id.localeCompare(a.id)).filter(r=>!after||r.recordedAt<after.recordedAt||r.recordedAt===after.recordedAt&&r.id<after.id);
    const page=records.slice(0,25),items=page.map(r=>project(p,r,s,asOf));
    const next=records.length>25?ref(p,{kind:'cursor',s,asOf,after:page.at(-1)}):null;
    const data={window:{start:s.start,end:s.end,asOf,timeBasis:'OBSERVED'},items,next};
    if(state.afterRead)await state.afterRead();if(state.version!==version)throw new LogReadError('LOG_QUERY_EXPIRED');if(state.fault)state.fault(data);return wrap(p,data);
   },
   async read(value){const p=await checked(),version=state.version;state.calls.push({workspace,kind:'detail'});const refData=recover(value,p,'entry'),r=state.records.find(r=>r.workspace===workspace&&r.id===refData.id);if(!r)throw new LogReadError('LOG_ENTRY_UNAVAILABLE');if(hash(r)!==refData.hash)throw new LogReadError('LOG_ENTRY_CHANGED');
    const item={...project(p,r,refData.s,refData.asOf),entryRef:value};if(state.afterRead)await state.afterRead();if(state.version!==version)throw new LogReadError('LOG_QUERY_EXPIRED');return wrap(p,{window:{start:refData.s.start,end:refData.s.end,asOf:refData.asOf,timeBasis:'OBSERVED'},item});
   },
  };
 }
 return {state,window,reader,references,rowKey};
}
