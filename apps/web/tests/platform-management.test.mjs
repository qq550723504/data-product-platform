import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {fixture,MemoryStore} from './helpers/oidc-fixture.mjs';
const require=createRequire(import.meta.url);
const {managementSelection,managementQuery,validManagement}=require('../.iam-tests/platform-management.js');
test('management RP is disjoint and transport allows only exact readonly workspace queries',async t=>{
 const f=await fixture(t,new MemoryStore(),'platform-management'),start=await f.iam.start(),session=await f.iam.callback(f.callback(start),start.key),client=await f.iam.managementClient(session.key);
 assert.deepEqual(client.principal.capabilities,['PLATFORM_MANAGEMENT_READ']);await assert.rejects(f.iam.acceptanceClient(session.key));
 const base=client.apiBaseUrl+'/api/v1/workspaces/'+client.principal.workspace_id+'/management/';let calls=0;
 f.state.businessHandler=async(req,res)=>{calls++;assert.equal(req.method,'GET');res.end('{}')};
 await client.request(base+'members?offset=25');assert.equal(calls,1);
 for(const [url,init]of [[base+'members',{method:'POST'}],[base+'roles',{headers:{Authorization:'Bearer forged'}}],[base+'audit',{body:'x'}],[base.replace(client.principal.workspace_id,'44444444-4444-4444-8444-444444444444')+'members',{}],[client.apiBaseUrl+'/api/v1/workspaces/'+client.principal.workspace_id+'/acceptance-context',{}]])await assert.rejects(client.request(url,init));
 assert.equal(calls,1);
 f.state.businessHandler=async(req,res)=>{calls++;f.state.revision='1:1:2';res.end('{}')};await assert.rejects(client.request(base+'members'));assert.equal(calls,2);
});
test('acceptance sessions cannot enter management and management confirms only its own profile',async t=>{
 const f=await fixture(t),start=await f.iam.start(),s=await f.iam.callback(f.callback(start),start.key);await assert.rejects(f.iam.managementClient(s.key));
 const m=await fixture(t,new MemoryStore(),'platform-management'),a=await m.iam.start(),one=await m.iam.callback(m.callback(a),a.key);m.state.principalOverride={profileVersion:'business-acceptance.v1'};await assert.rejects(m.iam.current(one.key));
});
test('management selectors preserve filters and reject ambiguous identity and paging inputs',()=>{
 const s=managementSelection('members',{offset:'25',q:'目标',state:'active',memberId:'grant-2'});assert.equal(managementQuery(s),'?offset=25&q=%E7%9B%AE%E6%A0%87&state=active&memberId=grant-2');assert.deepEqual(managementSelection('members',{q:'',state:''}),{offset:0});
 for(const q of [{offset:'01'},{offset:'10001'},{offset:['0','25']},{workspaceId:'foreign'},{state:'pending'},{q:'a\u0000b'}])assert.throws(()=>managementSelection('members',q));
});
test('management response never trusts another workspace, revision or unexpected source payload',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'};
 const raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:new Date().toISOString(),source:'SYNTHETIC_FIXTURE',writeEnabled:false,data:{items:[],total:0,offset:0,limit:25,selected:null,roleDefinitions:[{id:'viewer',name:'查询员',permissions:['management.members.read']}]}};
 assert.equal(validManagement(raw,p,'members',{offset:0}),true);
 for(const v of [{...raw,workspaceId:p.principal_id},{...raw,writeEnabled:true},{...raw,authorityRevision:'1:1:2'},{...raw,secret:'unexpected'},{...raw,data:{...raw.data,total:26}}])assert.equal(validManagement(v,p,'members',{offset:0}),false);
});

test('management permission allowlist rejects inherited Object property names',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'};
 for(const permission of ['toString','constructor','__proto__']){
  const raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:new Date().toISOString(),source:'SYNTHETIC_FIXTURE',writeEnabled:false,data:{items:[{id:'viewer',name:'查询员',permissions:[permission]}],policySource:'APPLICATION_CATALOG'}};
  assert.equal(validManagement(raw,p,'roles',{offset:0}),false,permission);
 }
});
