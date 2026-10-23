import {isIdentity} from './quality-attention';
import {RELEASE_STATUSES} from './core-history';
export type TraceItem={id:string;[key:string]:unknown};
export type ReleaseTrace={workspaceId:string;releaseId:string;releaseNo:string;status:string;productId:string;productVersionId:string;evidenceSnapshot?:{id:string;rootHash:string;integrityValid:boolean;createdAt:string};datasetVersions:TraceItem[];executions:TraceItem[];entityMappings:TraceItem[];entityMatchJobs:(TraceItem&{workspaceId:string})[];evidence:(TraceItem&{workspaceId:string})[];costEvents:(TraceItem&{workspaceId:string})[];auditEvents:TraceItem[]};
const groups=['datasetVersions','executions','entityMappings','entityMatchJobs','evidence','costEvents','auditEvents'] as const;
export function validReleaseTrace(value:unknown,ws:string,id:string,productId?:string):value is ReleaseTrace {
 if(!value||typeof value!=='object')return false;const t=value as ReleaseTrace;
 if(!isIdentity(ws)||!isIdentity(id)||t.workspaceId!==ws||t.releaseId!==id||!isIdentity(t.productId)||!isIdentity(t.productVersionId)||(productId!==undefined&&t.productId!==productId)||typeof t.releaseNo!=='string'||!RELEASE_STATUSES.includes(t.status))return false;
 let count=0;
 for(const key of groups){const rows=t[key];if(!Array.isArray(rows))return false;count+=rows.length;const seen=new Set();for(const row of rows){if(!row||!isIdentity(row.id)||seen.has(row.id))return false;seen.add(row.id);if(['evidence','costEvents','entityMatchJobs'].includes(key)&&row.workspaceId!==ws)return false;}}
 if(count>1000)return false;
 if(t.datasetVersions.some(v=>(typeof v.datasetId!=="string"||!isIdentity(v.datasetId))))return false;
 if(t.entityMappings.some(v=>typeof v.decisionId!=="string"||!isIdentity(v.decisionId)||typeof v.entityId!=="string"||!isIdentity(v.entityId)))return false;
 const snap=t.evidenceSnapshot;if(snap&&(!isIdentity(snap.id)||typeof snap.rootHash!=='string'||!snap.rootHash||typeof snap.integrityValid!=='boolean'))return false;
 return true;
}
export function traceSelection(q:Record<string,string|string[]|undefined>){
 const releaseId=q.releaseId===undefined?'':typeof q.releaseId==='string'&&isIdentity(q.releaseId)?q.releaseId.toLowerCase():null;
 const productId=q.productId===undefined?undefined:typeof q.productId==='string'&&isIdentity(q.productId)?q.productId.toLowerCase():null;
 const offset=q.returnOffset===undefined?'0':typeof q.returnOffset==='string'&&/^\d{1,8}$/.test(q.returnOffset)?q.returnOffset:null;
 const status=q.returnStatus===undefined?'':typeof q.returnStatus==='string'&&(q.returnStatus===''||RELEASE_STATUSES.includes(q.returnStatus))?q.returnStatus:null;
 if(releaseId===null||productId===null||offset===null||status===null)throw Error('INVALID_TRACE_REFERENCE');
 const back=new URLSearchParams({offset,status});if(releaseId)back.set('releaseId',releaseId);
 return {releaseId,productId,offset,status,back:'/core/releases?'+back};
}
