// Leader demo only: immutable synthetic GET responses, no database or provider.
import { createServer } from 'node:http';
import { pathToFileURL } from 'node:url';
import { createFixtureServer, ids } from '../browser/fixture-server.mjs';

export const taskId = 'a3630000-0000-4000-8000-000000000001';
export const diagnosisId = 'a3630000-0000-4000-8000-000000000002';
export const repairId = 'a3630000-0000-4000-8000-000000000003';
const stamp = '2026-10-25T01:00:00Z';
const common = {sourceVersionId:ids.goldVersion, purposeNote:'内部数据治理 · 合成样例', acceptanceText:'保留明确版本、发现项和证据引用；当前仅审阅，不执行', templateRef:'park/remediation/energy-quarantine-v1.yaml', templateVersion:'1', inputs:[], actions:[]};
// Display-only IDs/hashes. These are not command receipts or validated bindings.
const plans = [
  {...common,id:diagnosisId,ordinal:1,phase:'DIAGNOSIS',goal:'核对合成质量报告及负能耗发现项',contentHash:'1'.repeat(64),actions:[{ordinal:1,commandKind:'DETECT'}]},
  {...common,id:repairId,ordinal:2,phase:'REPAIR',goal:'审阅负能耗问题的修复范围（静态计划样例，未执行）',contentHash:'2'.repeat(64),sourceAssessmentId:ids.goldAssessment,findingId:ids.goldBinding,
    caseId:'a3630000-0000-4000-8000-000000000004', workflowVersionId:'a3630000-0000-4000-8000-000000000005',outputDatasetId:'a3630000-0000-4000-8000-000000000006',targetPeriod:'2026-09',inputs:['company','tax','energy','resolved'].map((inputName,i)=>({inputName,datasetVersionId:`a3630000-0000-4000-8000-00000000001${i}`})),actions:['ASSIGN_SELF','EXECUTE_REPAIR','RECHECK'].map((commandKind,i)=>({ordinal:i+1,commandKind}))}];
const task = {workspaceId:ids.workspace,id:taskId,revision:2,controlState:'ACTIVE',currentPlanRevisionId:repairId,createdAt:stamp};

export function createLeaderFixture() {
  const fixture = createFixtureServer('leader-demo-quality');
  return createServer((req,res)=>{
    const url = new URL(req.url,'http://127.0.0.1');
    const send = (status,value)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store','X-Demo-Source':'synthetic-readonly'});res.end(JSON.stringify(value));};
    // No public scenario controls, even GET. No successful command responses.
    if(req.method!=='GET') return send(405,{code:'READ_ONLY_PREVIEW'});
    if(url.pathname.startsWith('/__control/')) return send(404,{code:'NOT_FOUND'});
    if(url.pathname==='/__health')return send(200,{source:'synthetic-readonly',businessWrites:false,models:false,dispatch:false});
    if(url.pathname.includes('/agent-task')){
      if(req.headers.authorization!=='Bearer review-secret')return send(401,{code:'AUTHENTICATION_REQUIRED'});
      const root=`/api/v1/workspaces/${ids.workspace}/agent-tasks`;
      if(url.pathname===root){const offset=Number(url.searchParams.get('offset')??0);if(!Number.isSafeInteger(offset)||offset<0)return send(400,{code:'INVALID_PAGE'});return send(200,{items:offset===0?[task]:[],page:{offset,limit:25,total:1}});}
      if(url.pathname===root+'/'+taskId){const id=url.searchParams.get('planId')??repairId;const plan=plans.find(p=>p.id===id);if(!plan)return send(404,{code:'AGENT_OBJECT_NOT_FOUND'});return send(200,{task,plan,plans:plans.map(({id,ordinal,phase})=>({id,ordinal,phase})),planPage:{offset:0,total:2},approvals:[],actions:[]});}
      return send(404,{code:'AGENT_OBJECT_NOT_FOUND'});
    }
    fixture.emit('request',req,res);
  });
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){
  const server=createLeaderFixture();
  server.listen(4493,'127.0.0.1',()=>console.log('Synthetic GET-only demo fixture: 127.0.0.1:4493'));
  process.on('SIGTERM',()=>server.close(()=>process.exit(0)));
}
