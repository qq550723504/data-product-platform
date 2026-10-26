// Server-rendered projections. The records API owns current dual-capability checks;
// platform-management.v1 sessions continue to project only management read.
export const logServices = {APP_SERVICE:'应用服务',BACKGROUND_SERVICE:'后台处理'} as const;
export const logLevels = {INFO:'常规',WARN:'注意',ERROR:'异常',FATAL:'严重异常',UNKNOWN:'未分类'} as const;
export const logEvents = {READ_COMPLETED:'查询完成',ACCESS_DENIED:'访问被拒绝',READ_FAILED:'查询未完成',DEPENDENCY_UNAVAILABLE:'依赖服务暂不可用',UNCLASSIFIED:'未分类运行记录'} as const;
export type LogSelection = {start:string;end:string;service?:keyof typeof logServices;level?:keyof typeof logLevels;cursor?:string;entryRef?:string;focus?:string;from?:'configuration'};
export type LogPrincipal = {principal_id:string;workspace_id:string;revision:string;expires_at:string;capabilities:string[]};
export type LogWindow = {start:string;end:string;asOf:string;timeBasis:'OBSERVED'};
export type LogEntry = {rowKey:string;entryRef:string;recordedAt:string;occurredAt:string|null;service:keyof typeof logServices;level:keyof typeof logLevels;eventCode:keyof typeof logEvents};
export type LogEnvelope<T> = {profileVersion:'platform-management.v1';workspaceId:string;principalId:string;authorityRevision:string;evaluatedAt:string;source:'CONFIGURED'|'SYNTHETIC_FIXTURE';writeEnabled:false;data:T};
export interface RuntimeLogReader {
 confirm():Promise<LogPrincipal>;
 source():Promise<unknown>;
 query(selection:LogSelection):Promise<unknown>;
 read(entryRef:string):Promise<unknown>;
}
export class LogReadError extends Error {constructor(public code:string){super(code)}}
export type LogView = {selection?:LogSelection;redirect?:string;code?:string;result?:LogEnvelope<{window:LogWindow;items:LogEntry[];next:string|null}>;detail?:LogEnvelope<{window:LogWindow;item:LogEntry}>};
const exact=(v:object,keys:string)=>Object.keys(v).sort().join(',')===keys.split(',').sort().join(',');
const token=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9_-]{16,2048}$/.test(v);
const row=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9_-]{43}$/.test(v);
export function logTime(v:unknown):v is string {
 if(typeof v!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{9}Z$/.test(v))return false;
 const ms=Date.parse(v);return Number.isFinite(ms)&&new Date(ms).toISOString()===v.slice(0,23)+'Z';
}
const nanos=(v:string)=>BigInt(Date.parse(v.slice(0,19)+'Z'))*BigInt(1000000)+BigInt(v.slice(20,29));
export const logNow=(now=Date.now())=>new Date(now).toISOString().replace('Z','000000Z');
export function logSelection(q:Record<string,string|string[]|undefined>,now=Date.now()):LogSelection {
 for(const [k,v]of Object.entries(q))if(!['start','end','service','level','cursor','entryRef','focus','from'].includes(k)||typeof v!=='string'||!v&&!['service','level'].includes(k))throw new LogReadError('INVALID_LOG_QUERY');
 q=Object.fromEntries(Object.entries(q).filter(([,v])=>v!==''));
 if(!q.start&&!q.end){if(Object.keys(q).some(k=>k!=='from'))throw new LogReadError('INVALID_LOG_QUERY');return {...logSelection({start:logNow(now-3600000),end:logNow(now)},now),...(q.from==='configuration'?{from:'configuration' as const}:q.from?invalid():{})}}
 const {start,end}=q;if(!logTime(start)||!logTime(end)||nanos(start)>=nanos(end)||nanos(end)-nanos(start)>BigInt('86400000000000')||nanos(end)>nanos(logNow(now)))throw new LogReadError('INVALID_LOG_QUERY');
 if(q.service&&!Object.hasOwn(logServices,q.service as string)||q.level&&!Object.hasOwn(logLevels,q.level as string)||q.cursor&&!token(q.cursor)||q.entryRef&&!token(q.entryRef)||q.focus&&!row(q.focus)||q.from&&q.from!=='configuration')throw new LogReadError('INVALID_LOG_QUERY');
 return Object.fromEntries(Object.entries(q).filter(([,v])=>v!==undefined)) as LogSelection;
}
function invalid():never{throw new LogReadError('INVALID_LOG_QUERY')}
export function logHref(s:LogSelection,patch:Partial<LogSelection>={}){
 const q=new URLSearchParams();for(const [k,v]of Object.entries({...s,...patch}))if(v)q.set(k,v);return '/settings/logs?'+q;
}
export function logListHref(s:LogSelection,focus?:string){return logHref(s,{entryRef:undefined,focus:focus??s.focus})+(focus?'#log-'+focus:'')}
function envelope<T>(raw:unknown,p:LogPrincipal,maxBytes=262144):raw is LogEnvelope<T>{
 if(!raw||typeof raw!=='object')return false;const r=raw as LogEnvelope<T>;
 if(new TextEncoder().encode(JSON.stringify(raw)).length>maxBytes)return false;
 return exact(r,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')&&r.profileVersion==='platform-management.v1'&&r.workspaceId===p.workspace_id&&r.principalId===p.principal_id&&r.authorityRevision===p.revision&&Number.isFinite(Date.parse(r.evaluatedAt))&&['CONFIGURED','SYNTHETIC_FIXTURE'].includes(r.source)&&r.writeEnabled===false;
}
function windowValid(w:LogWindow,s:LogSelection,now:number){return !!w&&exact(w,'start,end,asOf,timeBasis')&&w.start===s.start&&w.end===s.end&&w.timeBasis==='OBSERVED'&&logTime(w.asOf)&&w.asOf<=logNow(now)}
function entryValid(e:LogEntry,s:LogSelection){return !!e&&exact(e,'rowKey,entryRef,recordedAt,occurredAt,service,level,eventCode')&&row(e.rowKey)&&token(e.entryRef)&&logTime(e.recordedAt)&&e.recordedAt>=s.start&&e.recordedAt<s.end&&(e.occurredAt===null||logTime(e.occurredAt))&&Object.hasOwn(logServices,e.service)&&Object.hasOwn(logLevels,e.level)&&Object.hasOwn(logEvents,e.eventCode)&&(!s.service||s.service===e.service)&&(!s.level||s.level===e.level)}
export function validLogList(raw:unknown,p:LogPrincipal,s:LogSelection,now=Date.now()):raw is NonNullable<LogView['result']>{
 try{if(!envelope<{window:LogWindow;items:LogEntry[];next:string|null}>(raw,p))return false;const d=raw.data;
  return !!d&&exact(d,'window,items,next')&&windowValid(d.window,s,now)&&Array.isArray(d.items)&&d.items.length<=25&&d.items.every(e=>entryValid(e,s))&&new Set(d.items.map(e=>e.rowKey)).size===d.items.length&&new Set(d.items.map(e=>e.entryRef)).size===d.items.length&&(d.next===null||token(d.next)&&d.items.length===25)&&d.items.every((e,i)=>i===0||d.items[i-1].recordedAt>=e.recordedAt)&&d.items.every(e=>e.recordedAt<=d.window.asOf);
 }catch{return false}
}
export function validLogDetail(raw:unknown,p:LogPrincipal,s:LogSelection,now=Date.now()):raw is NonNullable<LogView['detail']>{
 try{if(!envelope<{window:LogWindow;item:LogEntry}>(raw,p,65536))return false;const d=raw.data;return !!d&&exact(d,'window,item')&&windowValid(d.window,s,now)&&entryValid(d.item,s)&&d.item.entryRef===s.entryRef&&(!s.focus||d.item.rowKey===s.focus)&&d.item.recordedAt<=d.window.asOf}catch{return false}
}
function authority(p:LogPrincipal){
 if(!p||!p.principal_id||!p.workspace_id||!/^[1-9][0-9]*:[1-9][0-9]*:[1-9][0-9]*$/.test(p.revision)||!Number.isFinite(Date.parse(p.expires_at))||Date.parse(p.expires_at)<=Date.now())throw new LogReadError('AUTHENTICATION_REQUIRED');
 if(!p.capabilities.includes('PLATFORM_MANAGEMENT_READ'))throw new LogReadError('LOG_ACCESS_DENIED');
}
async function guarded<T>(reader:RuntimeLogReader,p:LogPrincipal,operation:()=>Promise<T>){
 authority(p);const data=await operation(),after=await reader.confirm();authority(after);
 if(after.principal_id!==p.principal_id||after.workspace_id!==p.workspace_id||after.revision!==p.revision)throw new LogReadError('AUTHENTICATION_REQUIRED');return data;
}
export async function loadRuntimeLogs(q:Record<string,string|string[]|undefined>,reader:RuntimeLogReader|null):Promise<LogView>{
 // Source-less production entry never creates a query or accepts a test-mode URL.
 if(!reader)return {code:'LOG_SOURCE_UNCONFIGURED'};
 let selection:LogSelection|undefined;
 try{
  const p=await reader.confirm();authority(p);
  const source=await guarded(reader,p,()=>reader.source());
  if(!envelope<{state:string;availability:string}>(source,p)||!source.data||!exact(source.data,'state,availability')||source.data.availability!=='NOT_CHECKED'||!['UNCONFIGURED','CONFIGURED','UNSUPPORTED'].includes(source.data.state))throw new LogReadError('LOG_SOURCE_UNAVAILABLE');
  if(source.data.state!=='CONFIGURED')return {code:source.data.state==='UNCONFIGURED'?'LOG_SOURCE_UNCONFIGURED':'LOG_SOURCE_UNSUPPORTED'};
  authority(p);selection=logSelection(q);
  if(!q.start)return {redirect:logHref(selection)};
  const raw=await guarded(reader,p,()=>selection!.entryRef?reader.read(selection!.entryRef):reader.query(selection!));
  if(selection.entryRef){if(!validLogDetail(raw,p,selection))throw new LogReadError('LOG_SOURCE_UNAVAILABLE');return {selection,detail:raw}}
  if(!validLogList(raw,p,selection))throw new LogReadError('LOG_SOURCE_UNAVAILABLE');return {selection,result:raw};
 }catch(e){
  if(!selection&&e instanceof LogReadError&&e.code==='INVALID_LOG_QUERY'&&q.start&&q.end){try{selection=logSelection(Object.fromEntries(Object.entries(q).filter(([k])=>['start','end','service','level','from'].includes(k))))}catch{/* Never echo invalid times or arbitrary return targets. */}}
  return {selection,code:e instanceof LogReadError?e.code:'LOG_SOURCE_UNAVAILABLE'};
 }
}

// Backend authorization is performed by every source/list/detail request. This
// adapter never manufactures a new capability in the existing RP profile.
export function runtimeLogAPI(client:{principal:LogPrincipal;apiBaseUrl:string;request:typeof fetch;confirm():Promise<LogPrincipal>}):RuntimeLogReader{
 const base=client.apiBaseUrl+'/api/v1/workspaces/'+client.principal.workspace_id+'/management/runtime-logs/';
 const request=async(path:string)=>{
  let response:Response;try{response=await client.request(base+path)}catch{throw new LogReadError('AUTHENTICATION_REQUIRED')}
  const raw=await response.json();if(!response.ok){const allowed=['INVALID_LOG_QUERY','LOG_QUERY_EXPIRED','LOG_ENTRY_CHANGED','LOG_ENTRY_UNAVAILABLE','LOG_WINDOW_EXPIRED','LOG_SOURCE_UNCONFIGURED','LOG_SOURCE_UNSUPPORTED','LOG_ACCESS_DENIED','AUTHENTICATION_REQUIRED'];throw new LogReadError(response.status===401?'AUTHENTICATION_REQUIRED':response.status===403?'LOG_ACCESS_DENIED':allowed.includes(raw?.code)?raw.code:'LOG_SOURCE_UNAVAILABLE')}return raw;
 };
 return {async confirm(){try{return await client.confirm()}catch{throw new LogReadError('AUTHENTICATION_REQUIRED')}},source:()=>request('source'),query:s=>{
  const q=new URLSearchParams({start:s.start,end:s.end});for(const k of ['service','level','cursor']as const)if(s[k])q.set(k,s[k]!);return request('entries?'+q);
 },read:entryRef=>{if(!token(entryRef))throw new LogReadError('INVALID_LOG_QUERY');return request('entries/'+entryRef)}};
}

// Used by the Next client and the isolated browser host. No local/session storage.
export function runtimeLogBrowserGuard(){
 const focus=()=>{if(location.hash.startsWith('#log-'))document.getElementById(location.hash.slice(1))?.focus()};
 const hide=()=>{document.querySelectorAll('[data-runtime-log-results]').forEach(el=>{el.replaceChildren()})};
 const show=(e:PageTransitionEvent)=>{if(e.persisted)location.reload();else focus()};
 focus();window.addEventListener('pagehide',hide);window.addEventListener('pageshow',show);
 return ()=>{window.removeEventListener('pagehide',hide);window.removeEventListener('pageshow',show)};
}
