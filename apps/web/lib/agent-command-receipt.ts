import {isIdentity} from './quality-attention';
export type PendingAgentCommand={path:string;key:string;body:Record<string,unknown>};
export type AgentReceipt={id:string;taskId:string;planRevisionId:string;taskRevision:number;approvalId?:string;actionId?:string};
type Reply={ok:boolean;status:number;json():Promise<unknown>};
type Result={state:'UNKNOWN'}|{state:'REJECTED';code:string}|{state:'CONFIRMED';receipt:AgentReceipt;readFailed:boolean};
const id=(v:unknown):v is string=>typeof v==='string'&&isIdentity(v);
const same=(a:unknown,b:unknown)=>id(a)&&id(b)&&a.toLowerCase()===b.toLowerCase();
export function matchingAgentReceipt(value:unknown,request:PendingAgentCommand):value is AgentReceipt{
 if(!value||typeof value!=='object')return false;const r=value as AgentReceipt;
 if(!id(r.id)||!id(r.taskId)||!id(r.planRevisionId)||!Number.isSafeInteger(r.taskRevision)||r.taskRevision<1)return false;
 if(r.approvalId!==undefined&&!id(r.approvalId)||r.actionId!==undefined&&!id(r.actionId))return false;
 if(request.path==='')return r.taskRevision===1&&r.approvalId===undefined&&r.actionId===undefined;
 const m=request.path.match(/^\/([0-9a-f-]+)\/(revisions|approvals|actions|pause|resume|stop)$/i);
 const revision=request.body.expectedRevision;
 if(!m||!same(m[1],r.taskId)||typeof revision!=='number'||!Number.isSafeInteger(revision)||revision<1||!Number.isSafeInteger(revision+1)||r.taskRevision!==revision+1)return false;
 if(m[2]==='revisions')return id(request.body.basePlanRevisionId)&&!same(request.body.basePlanRevisionId,r.planRevisionId)&&r.approvalId===undefined&&r.actionId===undefined;
 if(m[2]==='approvals')return same(request.body.planRevisionId,r.planRevisionId)&&id(r.approvalId)&&r.actionId===undefined;
 if(m[2]==='actions')return same(request.body.planRevisionId,r.planRevisionId)&&same(request.body.approvalId,r.approvalId)&&id(r.actionId);
 return r.approvalId===undefined&&r.actionId===undefined;
}
// One explicit attempt only. Confirmation is settled before independent reads;
// refresh failures cannot erase a valid command receipt or trigger another POST.
export async function submitAgentCommand(request:PendingAgentCommand,io:{post:(url:string,init:RequestInit)=>Promise<Reply>;confirmed:(receipt:AgentReceipt)=>void;refresh:(receipt:AgentReceipt)=>Promise<void>}):Promise<Result>{
 let receipt:AgentReceipt;
 try{
  const response=await io.post('/api/agent-tasks'+request.path,{method:'POST',redirect:'error',headers:{'Content-Type':'application/json','Idempotency-Key':request.key},body:JSON.stringify(request.body)});
  const data=await response.json();
  if(!response.ok){
   const code=data&&typeof data==='object'?(data as {error?:{code?:unknown}}).error?.code:undefined;
   if(response.status>=400&&response.status<500&&![408,425,429].includes(response.status)&&typeof code==='string'&&/^[A-Z][A-Z0-9_]+$/.test(code))return {state:'REJECTED',code};
   return {state:'UNKNOWN'};
  }
  if(!matchingAgentReceipt(data,request))return {state:'UNKNOWN'};
  receipt=data;
 }catch{return {state:'UNKNOWN'};}
 io.confirmed(receipt);
 try{await io.refresh(receipt);return {state:'CONFIRMED',receipt,readFailed:false};}
 catch{return {state:'CONFIRMED',receipt,readFailed:true};}
}
export async function refreshAgentViews(list:()=>Promise<unknown>,detail:()=>Promise<unknown>):Promise<void>{
 const results=await Promise.allSettled([list(),detail()]);
 if(results.some(r=>r.status==='rejected'))throw Error('AGENT_READ_REFRESH_FAILED');
}
