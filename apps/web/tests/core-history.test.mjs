import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {historySelection,historyHref,validCertificationHistory,validReleaseHistory}=require('../.history-tests/core-history.js');
const ws='11111111-1111-4111-8111-111111111111',version='4c208c9b-d4d7-42ad-a119-101637d99e77',id='d3667496-f421-497c-be69-e5dec6bb569b',other='33333333-3333-4333-8333-333333333333',time='2026-10-23T00:00:00Z';
const s=historySelection({versionId:version},true);
const cert={id,workspaceId:ws,datasetVersionId:version,qualityAssessmentId:id,decision:'CERTIFIED',reason:'historical decision',issuedAt:time,profile:{id,workspaceId:ws,name:'profile',version:'1'},blockers:[],dispositions:[{id:other,disposition:'REVOKED',effectiveAt:time,reason:'later fact'}]};
const history={workspaceId:ws,datasetVersionId:version,asOf:time,items:[cert],page:{limit:25,offset:0,total:1,anchorRevision:3}};
test('certification decision and revocation remain separate; reject cross-scope and changed anchors',()=>{
 assert.equal(validCertificationHistory(history,ws,s),true);
 assert.equal(validCertificationHistory({...history,items:[{...cert,workspaceId:other}]},ws,s),false);
 assert.equal(validCertificationHistory({...history,items:[{...cert,datasetVersionId:other}]},ws,s),false);
 assert.equal(validCertificationHistory(history,ws,{...s,asOf:time,anchorRevision:4}),false);
 assert.equal(validCertificationHistory(history,ws,{...s,asOf:'2026-10-24T00:00:00Z'}),false);
});
test('history pagination preserves asOf/revision, including zero anchor; invalid identities never become latest',()=>{
 assert.throws(()=>historySelection({versionId:version,offset:'25'},true));
 assert.throws(()=>historySelection({versionId:['x',version]},true));
 assert.throws(()=>historySelection({versionId:'VER-0002'},true));
 const href=historyHref('/core/certifications',{...s,asOf:time,anchorRevision:0,offset:25});
 const next=historySelection(Object.fromEntries(new URL(href,'http://local').searchParams),true);
 assert.equal(next.anchorRevision,0);assert.equal(next.asOf,time);assert.equal(next.offset,25);
});
test('release history is workspace-scoped and filtered; no invented publication time or fallback on malformed data',()=>{
 const row={id,productId:id,productVersionId:other,releaseNo:'r1',status:'DRAFT',createdAt:time,releaseNotes:''};
 const h={workspaceId:ws,items:[row],page:{limit:25,offset:0,total:1}};
 assert.equal(validReleaseHistory(h,ws,{offset:0,status:''}),true);
 assert.equal(validReleaseHistory(h,ws,{offset:0,status:'PUBLISHED'}),false);
 assert.equal(validReleaseHistory({...h,workspaceId:other},ws,{offset:0,status:''}),false);
 assert.throws(()=>historySelection({status:'SUSPENDED'}));
 assert.equal(validReleaseHistory({...h,items:[],page:{limit:25,offset:0,total:0}},ws,{offset:0,status:''}),true);
});

// Presentation unit tests: Badge is a primitive stub; no Core data is created.
const React=require('react'),{renderToStaticMarkup}=require('react-dom/server'),ts=require('typescript'),fs=require('node:fs');
require.extensions['.tsx']=(mod,file)=>mod._compile(ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,module:ts.ModuleKind.CommonJS}}).outputText,file);
const ui=require.resolve('../components/ui.tsx');require.cache[ui]={id:ui,filename:ui,loaded:true,exports:{Badge:({value})=>React.createElement('span',null,value)}};
const {CertificationRecords,ReleaseRecords}=require('../components/core-history-records.tsx');
test('nonempty historical presentation keeps original certified decision plus revocation, exact selection opens its own facts',()=>{
 const html=renderToStaticMarkup(React.createElement(CertificationRecords,{history,selected:id}));
 assert.match(html,/CERTIFIED/);assert.match(html,/REVOKED/);assert.match(html,/historical decision/);assert.match(html,/<details open=""/);
 const missing=renderToStaticMarkup(React.createElement(CertificationRecords,{history:{...history,items:[]},selected:other}));
 assert.match(missing,/UNKNOWN/);assert.doesNotMatch(missing,/historical decision/);
});
test('release presentation does not invent publish time or dataset membership; missing selected batch is explicit',()=>{
 const row={id,productId:id,productVersionId:other,releaseNo:'r1',status:'DRAFT',createdAt:time,releaseNotes:'frozen release note'};
 const html=renderToStaticMarkup(React.createElement(ReleaseRecords,{history:{workspaceId:ws,items:[row],page:{limit:25,offset:0,total:1}},selected:id}));
 assert.match(html,/DRAFT/);assert.match(html,/未记录发布时间/);assert.match(html,/未返回数据集成员/);assert.match(html,/frozen release note/);
 const missing=renderToStaticMarkup(React.createElement(ReleaseRecords,{history:{workspaceId:ws,items:[],page:{limit:25,offset:0,total:0}},selected:other}));assert.match(missing,/UNKNOWN/);
});

const {qualityCosts}=require('../.history-tests/quality-cost.js');
test('quality cost projection requires exact subject and workspace, removes opaque metadata and preserves unknown amount',()=>{
 const event={id,workspaceId:ws,activityId:other,costType:'QUALITY',quantity:2,unit:'rows',pricingMode:'UNPRICED',occurredAt:time,metadata:{secret:'never-render'}};
 const detail={id,workspaceId:ws,datasetVersionId:version,costEvents:[event]};
 const costs=qualityCosts(detail,ws,version,id);assert.equal(costs.length,1);assert.equal(costs[0].amount,undefined);assert.equal(costs[0].metadata,undefined);
 assert.equal(qualityCosts({...detail,id:other},ws,version,id),null);
 assert.equal(qualityCosts({...detail,costEvents:[{...event,workspaceId:other}]},ws,version,id),null);
 assert.equal(qualityCosts({...detail,costEvents:[event,event]},ws,version,id),null);
 assert.equal(qualityCosts({...detail,costEvents:undefined},ws,version,id),null);
 assert.deepEqual(qualityCosts({...detail,costEvents:[]},ws,version,id),[]);
 const zero=qualityCosts({...detail,costEvents:[{...event,amount:'0.0000',currency:'CNY'}]},ws,version,id);assert.equal(zero[0].amount,'0.0000');
});

const {validReleaseTrace,traceSelection}=require('../.history-tests/release-trace.js');
test('release trace rejects substituted roots, foreign children, duplicate identities and malformed lineage',()=>{
 const t={workspaceId:ws,releaseId:id,releaseNo:'r1',status:'PUBLISHED',productId:other,productVersionId:version,datasetVersions:[{id:version,datasetId:id}],executions:[],entityMappings:[],entityMatchJobs:[],evidence:[{id,workspaceId:ws}],costEvents:[],auditEvents:[]};
 assert.equal(validReleaseTrace(t,ws,id),true);
 assert.equal(validReleaseTrace({...t,workspaceId:other},ws,id),false);
 assert.equal(validReleaseTrace(t,ws,other),false);
 assert.equal(validReleaseTrace(t,ws,id,id),false);
 assert.equal(validReleaseTrace({...t,evidence:[{id,workspaceId:other}]},ws,id),false);
 assert.equal(validReleaseTrace({...t,evidence:[t.evidence[0],t.evidence[0]]},ws,id),false);
 assert.equal(validReleaseTrace({...t,datasetVersions:[{id:version}]},ws,id),false);
 assert.equal(validReleaseTrace({...t,evidenceSnapshot:{id,rootHash:'hash',integrityValid:'true'}},ws,id),false);
});
test('trace refresh and return retain exact release and filtered history; invalid refs never become latest',()=>{
 const q=traceSelection({releaseId:id,returnOffset:'25',returnStatus:'PUBLISHED'});
 const back=new URL(q.back,'http://local');assert.equal(back.searchParams.get('releaseId'),id);assert.equal(back.searchParams.get('offset'),'25');assert.equal(back.searchParams.get('status'),'PUBLISHED');
 assert.equal(new URL(traceSelection({}).back,'http://local').searchParams.has('releaseId'),false);
 for(const q of [{releaseId:[id,other]},{releaseId:'latest'},{returnOffset:'-1'},{returnStatus:'SUSPENDED'}])assert.throws(()=>traceSelection(q));
});

test('mapping history uses immutable decision identity; snapshot membership stays separate from related evidence',()=>{
 const mapping={id:other,decisionId:id,entityId:version};
 const t={workspaceId:ws,releaseId:id,releaseNo:'r1',status:'PUBLISHED',productId:other,productVersionId:version,datasetVersions:[],executions:[],entityMappings:[mapping,{...mapping,decisionId:other}],entityMatchJobs:[],evidence:[{id,workspaceId:ws},{id:version,workspaceId:ws}],costEvents:[],auditEvents:[],evidenceSnapshot:{id:version,rootHash:'a'.repeat(64),integrityValid:true,createdAt:time,items:[{evidenceId:id,category:'FROZEN'}]}};
 assert.equal(validReleaseTrace(t,ws,id),true); // One mutable mapping, two actual historical decisions.
 assert.equal(validReleaseTrace({...t,entityMappings:[mapping,mapping]},ws,id),false);
 assert.equal(validReleaseTrace({...t,evidenceSnapshot:{...t.evidenceSnapshot,items:[{evidenceId:other,category:'MISSING'}]}},ws,id),false);
 assert.equal(validReleaseTrace({...t,evidenceSnapshot:{...t.evidenceSnapshot,items:undefined}},ws,id),false);
});

test('Trace UUID equivalence accepts uppercase configured scopes and rejects case-variant duplicates',()=>{
 const scope='abcdefab-abcd-4abc-8abc-abcdefabcdef';
 const t={workspaceId:scope,releaseId:id,releaseNo:'r1',status:'PUBLISHED',productId:version,productVersionId:other,datasetVersions:[],executions:[],entityMappings:[],entityMatchJobs:[],evidence:[{id,workspaceId:scope.toUpperCase()}],costEvents:[],auditEvents:[],evidenceSnapshot:{id:other,rootHash:'hash',integrityValid:true,items:[{evidenceId:id.toUpperCase(),category:'FROZEN'}]}};
 assert.equal(validReleaseTrace(t,scope.toUpperCase(),id.toUpperCase(),version.toUpperCase()),true);
 assert.equal(validReleaseTrace({...t,workspaceId:other},scope.toUpperCase(),id),false);
 assert.equal(validReleaseTrace({...t,evidence:[{id,workspaceId:other}]},scope,id),false);
 assert.equal(validReleaseTrace({...t,evidence:[...t.evidence,{...t.evidence[0],id:id.toUpperCase()}]},scope,id),false);
 assert.equal(validReleaseTrace({...t,evidenceSnapshot:{...t.evidenceSnapshot,items:[...t.evidenceSnapshot.items,{evidenceId:id,category:'FROZEN'}]}},scope,id),false);
 assert.equal(validReleaseTrace(t,'invalid',id),false);
});

test('history binding references accept absent/null identities but reject object children before actual page render',async()=>{
 const release={id,productId:id,productVersionId:other,releaseNo:'r1',status:'DRAFT',createdAt:time,releaseNotes:''};
 const page={limit:25,offset:0,total:1};
 const certKeys=['rightsSnapshotId','effectiveRightsSnapshotId','complianceResultId','contractVersionId','evidenceSnapshotId'];
 const releaseKeys=['qualityResultId','rightsSnapshotId','complianceResultId','contractVersionId','evidenceSnapshotId'];
 for(const [certification,record,keys] of [[true,cert,certKeys],[false,release,releaseKeys]]){
  const h=value=>certification?{...history,items:[value]}:{workspaceId:ws,items:[value],page};
  const valid=value=>certification?validCertificationHistory(h(value),ws,s):validReleaseHistory(h(value),ws,{offset:0,status:''});
  for(const key of keys){
   for(const value of [undefined,null,id])assert.equal(valid({...record,[key]:value}),true,key);
   for(const value of [{},[],false,0,'not-a-uuid'])assert.equal(valid({...record,[key]:value}),false,key);
   const imports={
    'react/jsx-runtime':require('react/jsx-runtime'),
    './core-history-records':require('../components/core-history-records.tsx'),
    '@/components/ui':{PageHeader:({title})=>React.createElement('h1',null,title),EmptyState:()=>null},
    '@/lib/platform':{configuredWorkspaceId:()=>ws},
    '@/lib/quality-server':{qualityRead:async()=>h({...record,[key]:{}})},
    '@/lib/workbench-read':require('../.history-tests/workbench-read.js'),
    '@/lib/agent-quality-server':{readAgentContext:async()=>({dataset:{name:'known source'},version:{id:version,versionNo:1}})},
    '@/lib/agent-quality':{agentSelection:x=>x,validAgentContext:()=>true},
    '@/lib/core-history':require('../.history-tests/core-history.js'),
   };
   const code=ts.transpileModule(fs.readFileSync(new URL('../components/core-history-view.tsx',import.meta.url),'utf8'),{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,module:ts.ModuleKind.CommonJS}}).outputText;
   const mod={exports:{}};new Function('require','module','exports',code)(name=>{assert.ok(name in imports,name);return imports[name];},mod,mod.exports);
   const element=await mod.exports.default({certification,searchParams:Promise.resolve(certification?{versionId:version}:{})});
   const html=renderToStaticMarkup(element);assert.match(html,/UNKNOWN/);assert.match(html,/历史读取失败/);
   if(certification)assert.ok(html.includes(version));
  }
 }
});

test('archive return keeps exact snapshot and refuses malformed reference',()=>{
 assert.equal(traceSelection({releaseId:id,returnArchiveId:other}).archiveId,other);
 assert.throws(()=>traceSelection({releaseId:id,returnArchiveId:'latest'}));
 assert.throws(()=>traceSelection({releaseId:id,returnArchiveId:[id,other]}));
});
