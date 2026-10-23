import { isIdentity } from './quality-attention';
import { validPage } from './workbench-read';
import type { CertificationHistory, DatasetCertification, PageResult, ProductRelease } from './platform';

export const HISTORY_LIMIT=25;
export const RELEASE_STATUSES=['DRAFT','VALIDATING','READY','PUBLISHED','FAILED'];
export type HistorySelection={datasetId?:string;versionId?:string;certificationId?:string;releaseId?:string;offset:number;asOf?:string;anchorRevision?:number;status:string};
export function historySelection(q:Record<string,string|string[]|undefined>,certification=false):HistorySelection{
 const s:HistorySelection={offset:0,status:''};
 for(const key of ['datasetId','versionId','certificationId','releaseId'] as const){const v=q[key];if(v!==undefined){if(typeof v!=='string'||!isIdentity(v))throw Error('INVALID_REFERENCE');s[key]=v.toLowerCase();}}
 for(const key of ['offset','anchorRevision'] as const){const v=q[key];if(v!==undefined){if(typeof v!=='string'||!/^\d+$/.test(v)||!Number.isSafeInteger(Number(v))||Number(v)>2147483647)throw Error('INVALID_PAGE');s[key]=Number(v);}}
 if(q.asOf!==undefined){if(typeof q.asOf!=='string'||!/^\d{4}-\d\d-\d\dT.*(?:Z|[+-]\d\d:\d\d)$/.test(q.asOf)||!Number.isFinite(Date.parse(q.asOf)))throw Error('INVALID_TIME');s.asOf=q.asOf;}
 if(q.status!==undefined){if(typeof q.status!=='string'||(q.status!==''&&!RELEASE_STATUSES.includes(q.status)))throw Error('INVALID_STATUS');s.status=q.status;}
 if(certification&&!s.versionId)throw Error('VERSION_REQUIRED');
 if((s.offset>0||s.anchorRevision!==undefined)&&certification&&(!s.asOf||s.anchorRevision===undefined))throw Error('HISTORY_ANCHOR_REQUIRED');
 return s;
}
export function historyHref(base:string,s:Partial<HistorySelection>){const q=new URLSearchParams();for(const [k,v] of Object.entries(s))if(v!==undefined&&v!==''&&v!==0)q.set(k,String(v));if(s.anchorRevision===0)q.set('anchorRevision','0');return base+(q.size?'?'+q:'');}
const same=(a:string,b:string)=>isIdentity(a)&&isIdentity(b)&&a.toLowerCase()===b.toLowerCase();
const optionalIdentity=(x:unknown)=>x===undefined||x===null||(typeof x==='string'&&isIdentity(x));
const time=(x:string)=>typeof x==='string'&&Number.isFinite(Date.parse(x));
export function validCertification(c:DatasetCertification,ws:string,version:string):boolean{
 return same(c.workspaceId,ws)&&same(c.datasetVersionId,version)&&isIdentity(c.id)&&isIdentity(c.qualityAssessmentId)&&[c.rightsSnapshotId,c.effectiveRightsSnapshotId,c.complianceResultId,c.contractVersionId,c.evidenceSnapshotId].every(optionalIdentity)&&typeof c.decision==='string'&&typeof c.reason==='string'&&time(c.issuedAt)&&isIdentity(c.profile.id)&&same(c.profile.workspaceId,ws)&&typeof c.profile.name==='string'&&typeof c.profile.version==='string'&&Array.isArray(c.blockers)&&c.blockers.every(b=>typeof b.code==='string'&&typeof b.detail==='string')&&Array.isArray(c.dispositions)&&c.dispositions.every(d=>isIdentity(d.id)&&['REVOKED','SUPERSEDED'].includes(d.disposition)&&time(d.effectiveAt)&&typeof d.reason==='string');
}
export function validCertificationHistory(h:CertificationHistory,ws:string,s:HistorySelection){
 return same(h.workspaceId,ws)&&same(h.datasetVersionId,s.versionId!)&&time(h.asOf)&&(!s.asOf||Date.parse(h.asOf)===Date.parse(s.asOf))&&Number.isSafeInteger(h.page.anchorRevision)&&h.page.anchorRevision>=0&&(s.anchorRevision===undefined||s.anchorRevision===h.page.anchorRevision)&&validPage(h,HISTORY_LIMIT,s.offset,c=>validCertification(c,ws,s.versionId!));
}
export type ReleaseHistory=PageResult<ProductRelease>&{workspaceId:string};
export function validReleaseHistory(h:ReleaseHistory,ws:string,s:HistorySelection){return same(h.workspaceId,ws)&&validPage(h,HISTORY_LIMIT,s.offset,r=>isIdentity(r.id)&&isIdentity(r.productId)&&isIdentity(r.productVersionId)&&typeof r.releaseNo==='string'&&RELEASE_STATUSES.includes(r.status)&&(!s.status||r.status===s.status)&&time(r.createdAt)&&(!r.releasedAt||time(r.releasedAt))&&[r.qualityResultId,r.rightsSnapshotId,r.complianceResultId,r.contractVersionId,r.evidenceSnapshotId].every(optionalIdentity)&&typeof r.releaseNotes==='string');}
