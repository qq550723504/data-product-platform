import {isIdentity} from './quality-attention';
import {RELEASE_STATUSES} from './core-history';
export type TraceItem={id:string;[key:string]:unknown};
export type ReleaseTrace={workspaceId:string;releaseId:string;releaseNo:string;status:string;productId:string;productVersionId:string;evidenceSnapshot?:{id:string;rootHash:string;integrityValid:boolean;createdAt:string;items:{evidenceId:string;category:string}[]};datasetVersions:TraceItem[];executions:TraceItem[];entityMappings:TraceItem[];entityMatchJobs:(TraceItem&{workspaceId:string})[];evidence:(TraceItem&{workspaceId:string})[];costEvents:(TraceItem&{workspaceId:string})[];auditEvents:TraceItem[]};
const groups=['datasetVersions','executions','entityMappings','entityMatchJobs','evidence','costEvents','auditEvents'] as const;
export const sameTraceIdentity=(a:unknown,b:unknown):boolean=>typeof a==='string'&&typeof b==='string'&&isIdentity(a)&&isIdentity(b)&&a.toLowerCase()===b.toLowerCase();
export function validReleaseTrace(value:unknown,ws:string,id:string,productId?:string):value is ReleaseTrace {
 if(!value||typeof value!=='object')return false;const t=value as ReleaseTrace;
 if(!sameTraceIdentity(t.workspaceId,ws)||!sameTraceIdentity(t.releaseId,id)||typeof t.productId!=='string'||!isIdentity(t.productId)||typeof t.productVersionId!=='string'||!isIdentity(t.productVersionId)||(productId!==undefined&&!sameTraceIdentity(t.productId,productId))||typeof t.releaseNo!=='string'||!RELEASE_STATUSES.includes(t.status))return false;
 let count=0;
 for(const key of groups){const rows=t[key];if(!Array.isArray(rows))return false;count+=rows.length;const seen=new Set();for(const row of rows){if(!row||typeof row.id!=='string'||!isIdentity(row.id))return false;const identity=key==='entityMappings'?row.decisionId:row.id;if(typeof identity!=='string'||!isIdentity(identity)||seen.has(identity.toLowerCase()))return false;seen.add(identity.toLowerCase());if(['evidence','costEvents','entityMatchJobs'].includes(key)&&!sameTraceIdentity(row.workspaceId,ws))return false;}}
 if(count>1000)return false;
 if(t.datasetVersions.some(v=>(typeof v.datasetId!=="string"||!isIdentity(v.datasetId))))return false;
 if(t.entityMappings.some(v=>typeof v.decisionId!=="string"||!isIdentity(v.decisionId)||typeof v.entityId!=="string"||!isIdentity(v.entityId)))return false;
 const snap=t.evidenceSnapshot;if(snap&&(!isIdentity(snap.id)||typeof snap.rootHash!=='string'||!snap.rootHash||typeof snap.integrityValid!=='boolean'||!Array.isArray(snap.items)))return false;
 if(snap){const seen=new Set();for(const item of snap.items){if(!item||typeof item.evidenceId!=='string'||!isIdentity(item.evidenceId)||typeof item.category!=='string'||!item.category||seen.has(item.category+'\0'+item.evidenceId.toLowerCase())||!t.evidence.some(e=>sameTraceIdentity(e.id,item.evidenceId)))return false;seen.add(item.category+'\0'+item.evidenceId.toLowerCase());}if(count+snap.items.length>1000)return false;}
 return true;
}
export function traceSelection(q:Record<string,string|string[]|undefined>){
 const archiveId=q.returnArchiveId===undefined?undefined:typeof q.returnArchiveId==='string'&&isIdentity(q.returnArchiveId)?q.returnArchiveId.toLowerCase():null;
 if(archiveId===null)throw Error('INVALID_ARCHIVE_REFERENCE');
 const releaseId=q.releaseId===undefined?'':typeof q.releaseId==='string'&&isIdentity(q.releaseId)?q.releaseId.toLowerCase():null;
 const productId=q.productId===undefined?undefined:typeof q.productId==='string'&&isIdentity(q.productId)?q.productId.toLowerCase():null;
 const offset=q.returnOffset===undefined?'0':typeof q.returnOffset==='string'&&/^\d{1,8}$/.test(q.returnOffset)?q.returnOffset:null;
 const status=q.returnStatus===undefined?'':typeof q.returnStatus==='string'&&(q.returnStatus===''||RELEASE_STATUSES.includes(q.returnStatus))?q.returnStatus:null;
 if(releaseId===null||productId===null||offset===null||status===null)throw Error('INVALID_TRACE_REFERENCE');
 const back=new URLSearchParams({offset,status});if(releaseId)back.set('releaseId',releaseId);
 return {releaseId,productId,offset,status,archiveId,back:'/core/releases?'+back};
}
