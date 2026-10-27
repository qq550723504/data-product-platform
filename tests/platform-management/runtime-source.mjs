// Test-only source with no collector, database, business command or real log I/O.
export function runtimeSource(p){
 const a='11111111-1111-4111-8111-111111111111',b='12121212-1212-4212-8212-121212121212',held=new Set();
 const at=new Date(Date.now()-60000).toISOString().slice(0,20)+'123456789Z';
 const entries=[a,b].flatMap((ws,w)=>Array.from({length:58},(_,i)=>({workspaceId:ws,id:`${w?'B':'A'}-${String(i).padStart(3,'0')}`,recordedAt:at.slice(0,20)+(i<28?'123456789':'123456788')+'Z',occurredAt:i===57?null:at,service:i%2?'APP_SERVICE':'BACKGROUND_SERVICE',level:i%3?'INFO':'WARN',eventCode:i%5?'READ_COMPLETED':'READ_FAILED'})));
 const state={holdLogRead:false,logWaiting:0,logReads:0,logFail:false,logForeign:false,logDuplicate:false,logPrivate:false,logIncomplete:false,logOversize:false,logMissing:'',logChanged:''};
 const json=(res,v,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(v))};
 return {state,control(v){for(const k of ['holdLogRead','logFail','logForeign','logDuplicate','logPrivate','logIncomplete','logOversize'])if(typeof v[k]==='boolean')state[k]=v[k];for(const k of ['logMissing','logChanged'])if(typeof v[k]==='string'&&(!v[k]||/^[AB]-\d{3}$/.test(v[k])))state[k]=v[k];if(!state.holdLogRead)for(const release of [...held])release()},
 async handle(req,res,u){
  if(u.pathname!=='/_runtime-logs')return false;
  if(req.method!=='GET'||req.headers.authorization!=='Bearer '+p.readCredential){json(res,{},403);return true}
  const ws=u.searchParams.get('workspaceId');if(![a,b].includes(ws)){json(res,{},403);return true}state.logReads++;
  if(state.holdLogRead){state.logWaiting++;await new Promise(resolve=>{const release=()=>{clearTimeout(timer);held.delete(release);res.off('close',release);resolve()},timer=setTimeout(release,5000);held.add(release);res.on('close',release)});state.logWaiting--}
  if(res.destroyed)return true;if(state.logFail){json(res,{error:'PRIVATE_RUNTIME_PROVIDER_ERROR'},503);return true}
  const records=entries.filter(r=>r.workspaceId===ws&&r.id!==state.logMissing).map(r=>({...r,...r.id===state.logChanged?{level:'FATAL'}:{}}));
  if(u.searchParams.has('id')){const record=records.find(r=>r.id===u.searchParams.get('id'));if(!record){json(res,{},404);return true}if(state.logPrivate)record.body='PRIVATE_RUNTIME_BODY';json(res,record);return true}
  const q=u.searchParams,after=q.get('afterTime'),afterId=q.get('afterId'),items=records.filter(r=>r.recordedAt>=q.get('start')&&r.recordedAt<q.get('end')&&r.recordedAt<=q.get('asOf')&&(!q.has('service')||r.service===q.get('service'))&&(!q.has('level')||r.level===q.get('level'))&&(!after||r.recordedAt<after||r.recordedAt===after&&r.id<afterId)).sort((x,y)=>y.recordedAt.localeCompare(x.recordedAt)||y.id.localeCompare(x.id)).slice(0,26);
  if(state.logForeign&&items.length)items[0].workspaceId=ws===a?b:a;
  if(state.logDuplicate&&items.length>1)items[1]={...items[0]};
  if(state.logPrivate&&items.length)items[0].body='PRIVATE_RUNTIME_BODY';
  if(state.logOversize&&items.length)items[0].body='PRIVATE_RUNTIME_BODY'.repeat(20000);
  json(res,{items,complete:!state.logIncomplete});return true;
 }};
}
