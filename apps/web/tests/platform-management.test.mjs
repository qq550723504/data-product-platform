import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {fixture,MemoryStore} from './helpers/oidc-fixture.mjs';
const require=createRequire(import.meta.url);
const {managementSelection,managementQuery,managementPageQuery,workspaceQueryHref,organizationQueryHref,configurationQueryHref,managementMemberListHref,managementMemberDetailHref,managementUserListHref,managementUserDetailHref,validManagement,validUserManagement,validWorkspaceManagement,validOrganizationManagement,validConfigurationManagement}=require('../.iam-tests/platform-management.js');
test('management RP is disjoint and transport allows only exact readonly workspace queries',async t=>{
 const f=await fixture(t,new MemoryStore(),'platform-management'),start=await f.iam.start(),session=await f.iam.callback(f.callback(start),start.key),client=await f.iam.managementClient(session.key);
 assert.deepEqual(client.principal.capabilities,['PLATFORM_MANAGEMENT_READ']);await assert.rejects(f.iam.acceptanceClient(session.key));
 const base=client.apiBaseUrl+'/api/v1/workspaces/'+client.principal.workspace_id+'/management/';let calls=0;
 f.state.businessHandler=async(req,res)=>{calls++;assert.equal(req.method,'GET');res.end('{}')};
 await client.request(base+'members?offset=25');await client.request(base+'users?userId=native-user');await client.request(base+'workspace');await client.request(base+'organization');await client.request(base+'configuration');assert.equal(calls,5);
 for(const [url,init]of [[base+'members',{method:'POST'}],[base+'roles',{headers:{Authorization:'Bearer forged'}}],[base+'audit',{body:'x'}],[base.replace(client.principal.workspace_id,'44444444-4444-4444-8444-444444444444')+'members',{}],[client.apiBaseUrl+'/api/v1/workspaces/'+client.principal.workspace_id+'/acceptance-context',{}]])await assert.rejects(client.request(url,init));
 assert.equal(calls,5);
 f.state.businessHandler=async(req,res)=>{calls++;f.state.revision='1:1:2';res.end('{}')};await assert.rejects(client.request(base+'configuration'));assert.equal(calls,6);
});

test('configuration navigation retains its fixed origin through existing query flows without API parameters',()=>{
 for(const kind of ['workspaces','organizations','members','users','roles','audit']){const u=new URL(configurationQueryHref(kind),'https://app.invalid'),s=managementSelection(kind,Object.fromEntries(u.searchParams));assert.equal(s.from,'configuration');assert.equal(managementQuery(s),'')}
 assert.equal(workspaceQueryHref('members','configuration'),'/settings/members?from=configuration');assert.equal(organizationQueryHref('users','configuration'),'/settings/users?from=configuration');
 for(const s of [{offset:25,q:'目标',role:'viewer',state:'active',from:'configuration'}])for(const href of [managementMemberListHref(s,'grant'),managementMemberDetailHref(s,'grant'),managementUserListHref(s,'native'),managementUserDetailHref(s,'native')])assert.equal(new URL(href,'https://app.invalid').searchParams.get('from'),'configuration');
 for(const q of [{key:'DATABASE_URL'},{workspaceId:'foreign'},{from:'configuration'},{from:'//external.invalid'}])assert.throws(()=>managementSelection('configuration',q));
});

test('configuration DTO accepts only narrow public states and rejects secrets false health or global policy claims',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'};
 const data={scope:'CURRENT_WORKSPACE_MANAGEMENT',mode:'QUERY_ONLY',directory:{state:'UNCONFIGURED',source:'UNCONFIGURED',availability:'NOT_CHECKED'},queries:{workspace:true,organization:false,members:false,users:false,roles:false,audit:false},rolePolicySource:null},raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:new Date().toISOString(),source:'SYNTHETIC_FIXTURE',writeEnabled:false,data};assert.equal(validConfigurationManagement(raw,p),true);
 for(const patch of [{scope:'GLOBAL_PLATFORM'},{mode:'WRITE_ENABLED'},{databaseURL:'PRIVATE'},{clientSecret:'PRIVATE'},{rawEnvironment:{}},{directory:{...data.directory,availability:'HEALTHY'}},{directory:{...data.directory,organizationId:'PRIVATE'}},{queries:{...data.queries,users:true}},{rolePolicySource:'APPLICATION_CATALOG'}])assert.equal(validConfigurationManagement({...raw,data:{...data,...patch}},p),false);
 const configured={...data,directory:{state:'CONFIGURED',source:'CONFIGURED',availability:'NOT_CHECKED'},queries:{workspace:true,organization:true,members:true,users:true,roles:true,audit:true},rolePolicySource:'APPLICATION_CATALOG'};assert.equal(validConfigurationManagement({...raw,data:configured},p),true);
 for(const patch of [{workspaceId:p.principal_id},{principalId:p.workspace_id},{authorityRevision:'1:1:2'},{writeEnabled:true}])assert.equal(validConfigurationManagement({...raw,...patch},p),false);
});

test('organization origin preserves list conditions and rows without entering APIs or selecting another organization',()=>{
 for(const kind of ['members','users']){const u=new URL(organizationQueryHref(kind),'https://app.invalid');assert.equal(u.pathname,'/settings/'+kind);const s=managementSelection(kind,Object.fromEntries(u.searchParams));assert.equal(s.from,'organization');assert.equal(managementQuery(s),'')}
 const s={offset:25,q:'目标',state:'active',from:'organization'};
 for(const href of [managementMemberListHref(s,'grant'),managementMemberDetailHref(s,'grant'),managementUserListHref(s,'native'),managementUserDetailHref(s,'native')])assert.equal(new URL(href,'https://app.invalid').searchParams.get('from'),'organization');
 for(const q of [{organizationId:'foreign'},{from:'organization'},{from:'//foreign.invalid'},{offset:'25'}])assert.throws(()=>managementSelection('organizations',q));
 assert.throws(()=>managementSelection('members',{from:['organization','workspace']}));
});

test('organization DTO rejects foreign identity unexpected facts hierarchy and guessed state while preserving absent fields',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'};
 const data={organization:{id:'org',name:null,state:'active',createdAt:null,changedAt:null},departmentHierarchyState:'UNCONFIGURED'},raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:new Date().toISOString(),source:'SYNTHETIC_FIXTURE',writeEnabled:false,data};assert.equal(validOrganizationManagement(raw,p),true);
 for(const patch of [{state:'unknown'},{name:' '},{primaryDomain:'PRIVATE'},{createdAt:'not-a-date'},{parentId:'guessed-parent'}])assert.equal(validOrganizationManagement({...raw,data:{...data,organization:{...data.organization,...patch}}},p),false);
 assert.equal(validOrganizationManagement({...raw,data:{...data,departmentHierarchyState:'CONFIGURED'}},p),false);assert.equal(validOrganizationManagement({...raw,workspaceId:p.principal_id},p),false);assert.equal(validOrganizationManagement({...raw,authorityRevision:'2:1:1'},p),false);assert.equal(validOrganizationManagement({...raw,writeEnabled:true},p),false);
});

test('workspace origin is a fixed navigation marker excluded from API and restored across existing pages',()=>{
 for(const kind of ['members','users','roles','audit']){const u=new URL(workspaceQueryHref(kind),'https://app.invalid');assert.equal(u.pathname,'/settings/'+kind);const s=managementSelection(kind,Object.fromEntries(u.searchParams));assert.equal(s.from,'workspace');assert.equal(managementQuery(s),'');assert.equal(managementPageQuery(s),'?from=workspace')}
 const s={offset:25,q:'目标',role:'viewer',state:'active',from:'workspace'};
 for(const href of [managementMemberListHref(s,'grant-1'),managementMemberDetailHref(s,'grant-1'),managementUserListHref(s,'user-1'),managementUserDetailHref(s,'user-1')])assert.equal(new URL(href,'https://app.invalid').searchParams.get('from'),'workspace');
 assert.equal(managementQuery(s).includes('from'),false);assert.throws(()=>managementSelection('members',{from:'https://foreign.invalid'}));assert.throws(()=>managementSelection('workspaces',{workspaceId:'foreign'}));assert.throws(()=>managementSelection('workspaces',{from:'workspace'}));
});

test('workspace facts distinguish configured queries from availability and refuse invented lifecycle or scope',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'},now=new Date().toISOString();
 const data={recordState:'UNCONFIGURED',confirmationExpiresAt:new Date(Date.now()+60000).toISOString(),directory:{state:'UNCONFIGURED',source:'UNCONFIGURED',organizationId:null,projectId:null,availability:'NOT_CHECKED'},queries:{members:false,users:false,roles:false,audit:false}};
 const raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:now,source:'CONFIGURED',writeEnabled:false,data};assert.equal(validWorkspaceManagement(raw,p),true);
 for(const patch of [{recordState:'ACTIVE'},{owner:'guessed'},{confirmationExpiresAt:now},{directory:{...data.directory,availability:'HEALTHY'}},{directory:{...data.directory,organizationId:'guessed-org'}},{queries:{...data.queries,members:true}}])assert.equal(validWorkspaceManagement({...raw,data:{...data,...patch}},p),false);
 const configured={...data,directory:{state:'CONFIGURED',source:'SYNTHETIC_FIXTURE',organizationId:'org',projectId:'app',availability:'NOT_CHECKED'},queries:{members:true,users:true,roles:true,audit:true}};assert.equal(validWorkspaceManagement({...raw,data:configured},p),true);
 for(const patch of [{workspaceId:p.principal_id},{authorityRevision:'2:1:1'},{writeEnabled:true},{principalId:p.workspace_id}])assert.equal(validWorkspaceManagement({...raw,...patch,data:configured},p),false);
});

test('user directory navigation preserves search page and row with no foreign selectors',()=>{
 const s={offset:25,q:'成员 & 搜索',memberId:'grant',role:'viewer',cursor:'old',userId:'old'};
 const detail=new URL(managementUserDetailHref(s,'native-user'),'https://console.example.invalid');
 assert.deepEqual(managementSelection('users',Object.fromEntries(detail.searchParams)),{offset:25,q:s.q,userId:'native-user'});
 const list=new URL(managementUserListHref(s,'native-user'),detail);assert.equal(list.hash,'#user-native-user');assert.deepEqual(managementSelection('users',Object.fromEntries(list.searchParams)),{offset:25,q:s.q});
 for(const q of [{role:'viewer'},{memberId:'grant'},{returnTo:'//external.invalid'},{userId:'../x'},{userId:['one','two']}])assert.throws(()=>managementSelection('users',q));
});

test('user DTO separates account member and canonical identity facts and fails closed on wrong scope or private fields',()=>{
 const p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1'},at=new Date().toISOString();
 const m={id:'grant',userId:'native',organizationId:'org',projectId:'app',displayName:'Person',loginName:'login',roles:['viewer'],state:'inactive',createdAt:at,changedAt:at,permissions:['management.users.read']};
 const selected={profile:{id:'native',organizationId:'org',username:'login',displayName:'Person',accountState:'active',createdAt:at,changedAt:at},memberships:[m],identity:{workspaceId:p.workspace_id,state:'UNCONFIGURED',actorId:null,revision:'',basisRef:'',expiresAt:null}};
 const raw={profileVersion:'platform-management.v1',workspaceId:p.workspace_id,principalId:p.principal_id,authorityRevision:p.revision,evaluatedAt:at,source:'SYNTHETIC_FIXTURE',writeEnabled:false,data:{items:[{id:'native',displayName:'Person',loginName:'login',membershipCount:1}],total:1,offset:0,limit:25,selected,roleDefinitions:[{id:'viewer',name:'查询员',permissions:['management.users.read']}]}};
 assert.equal(validUserManagement(raw,p,{offset:0,userId:'native'}),true);
 const linked={...selected.identity,state:'LINKED',actorId:'44444444-4444-4444-8444-444444444444',revision:'2:3:4',basisRef:'explicit-source',expiresAt:new Date(Date.now()+300000).toISOString()};
 assert.equal(validUserManagement({...raw,data:{...raw.data,selected:{...selected,identity:linked}}},p,{offset:0,userId:'native'}),true);
 for(const selectedChange of [{profile:{...selected.profile,accountState:'unknown'}},{profile:{...selected.profile,email:'PRIVATE'}},{memberships:[{...m,organizationId:'foreign'}]},{identity:{...linked,workspaceId:p.principal_id}},{identity:{...linked,actorId:'native'}},{identity:{...selected.identity,actorId:linked.actorId}}])assert.equal(validUserManagement({...raw,data:{...raw.data,selected:{...selected,...selectedChange}}},p,{offset:0,userId:'native'}),false);
 assert.equal(validUserManagement({...raw,data:{...raw.data,items:[{...raw.data.items[0],accountState:'active'}]}},p,{offset:0,userId:'native'}),false);
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

test('role member navigation restores explicit conditions page and row without arbitrary return targets',()=>{
 const selection={offset:25,q:'空间甲 & 成员',role:'management_viewer',state:'active',memberId:'old',cursor:'unrelated'};
 const detail=new URL(managementMemberDetailHref(selection,'grant:a-055'),'https://console.example.invalid');
 assert.equal(detail.pathname,'/settings/members');assert.equal(detail.searchParams.has('cursor'),false);
 const parsed=managementSelection('members',Object.fromEntries(detail.searchParams));
 assert.deepEqual(parsed,{offset:25,q:selection.q,role:selection.role,state:selection.state,memberId:'grant:a-055'});
 const list=new URL(managementMemberListHref(parsed,parsed.memberId),detail);
 assert.equal(list.origin,detail.origin);assert.equal(list.searchParams.has('memberId'),false);assert.equal(list.searchParams.has('cursor'),false);
 assert.equal(decodeURIComponent(list.hash),'#member-grant:a-055');
 assert.deepEqual(managementSelection('members',Object.fromEntries(list.searchParams)),{offset:25,q:selection.q,role:selection.role,state:selection.state});
 assert.equal(managementMemberListHref({offset:0,role:'audit_viewer'}),'/settings/members?role=audit_viewer');
 for(const member of ['//other.invalid','../member','grant#fragment'])assert.throws(()=>managementMemberDetailHref(selection,member));
 assert.throws(()=>managementSelection('members',{returnTo:'https://other.invalid'}));
});
