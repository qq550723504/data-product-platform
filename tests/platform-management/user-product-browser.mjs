// Actual cmd/api + private product profile + real RP/PG; all sources synthetic.
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {reloadReadOnly} from '../browser/readonly-navigation.mjs';
if(process.env.MANAGEMENT_USER_PRODUCT_FIXTURE_ACK!=='ISOLATED_USER_PRODUCT_READONLY')throw Error('independent directory product fixture required');
const fixture=new URL('../../.data/management-user-product-20261027/',import.meta.url),p=JSON.parse(readFileSync(new URL('profile.json',fixture),'utf8'));
assert.equal(p.userProduct,true);assert.equal(p.issuer,'https://localhost:19220');assert.equal(p.origin,'https://localhost:4420');assert.equal(p.database,'management_user_product_20261027');
const dbURL=new URL(p.adminDSN);assert.equal(dbURL.hostname,'127.0.0.1');assert.equal(dbURL.port,'25620');assert.equal(dbURL.pathname,'/'+p.database);
const output=process.env.USER_PRODUCT_BROWSER_OUTPUT??'management-user-product-browser-20261027';assert.match(output,/^management-user-product-browser-[a-z0-9-]+$/);
const out=new URL('../../.data/'+output+'/',import.meta.url);if(existsSync(out))throw Error('previous browser evidence retained');mkdirSync(new URL('screenshots/',out),{recursive:true});
const require=createRequire(new URL('../browser/package.json',import.meta.url)),{chromium,request,expect}=require('@playwright/test'),{Pool}=createRequire(new URL('../../apps/web/package.json',import.meta.url))('pg');
const channel=process.env.MANAGEMENT_BROWSER_CHANNEL??'msedge';assert.ok(['msedge','chromium'].includes(channel));
const browser=await chromium.launch({...channel==='msedge'?{channel}:{},headless:true}),http=await request.newContext({ignoreHTTPSErrors:true}),db=new Pool({connectionString:p.adminDSN,ssl:{ca:readFileSync(p.cert),rejectUnauthorized:true}}),contexts=[],results=[],errors=[],writes=[],external=[];
const a='11111111-1111-4111-8111-111111111111',b='12121212-1212-4212-8212-121212121212',c='13131313-1313-4313-8313-131313131313',actor='83000000-0000-4000-8000-000000000001',api='https://localhost:18620',prefix=ws=>api+'/api/v1/workspaces/'+ws+'/management/';
let active='setup',page,detailURL;
async function control(data={}){const r=await http.post(p.issuer+'/_control',{headers:{'x-fixture-control':p.control},data,maxRedirects:0});assert.equal(r.status(),200);return r.json()}
async function token(subject='admin-a'){return (await control({issueSubject:subject})).testAccessToken}
async function query(ws,path,t,status=200){const r=await http.get(prefix(ws)+path,{headers:{Authorization:'Bearer '+t},maxRedirects:0});assert.equal(r.status(),status,'product HTTP status');assert.equal(r.headers()['cache-control'],'private, no-store');const d=await r.json();if(status!==200)assert.equal(Object.hasOwn(d,'data'),false);return d}
async function login(pg,label='查询人员 A',ws=a){await pg.goto(p.origin+'/auth/login?workspaceId='+ws);await pg.getByRole('button',{name:'使用统一身份登录',exact:true}).click();await pg.getByRole('button',{name:label,exact:true}).click();await pg.waitForURL(p.origin+'/settings',{waitUntil:'load'});await pg.goto(p.origin+'/settings')}
async function person(label='查询人员 A',ws=a){const context=await browser.newContext({ignoreHTTPSErrors:true,serviceWorkers:'block',viewport:{width:1600,height:950}});contexts.push(context);await context.route('**/*',route=>{const r=route.request(),u=new URL(r.url());if(![p.origin,p.issuer].includes(u.origin)){external.push(u.origin);return route.abort()}if(u.origin===p.origin&&!['GET','HEAD'].includes(r.method())&&!u.pathname.startsWith('/auth/')){writes.push(u.pathname);return route.abort()}return route.continue()});const pg=await context.newPage();pg.on('pageerror',e=>errors.push({case:active,path:new URL(pg.url()).pathname,message:e.message}));pg.on('crash',()=>errors.push({case:active,message:'PAGE_CRASH'}));await login(pg,label,ws);return pg}
const rows=pg=>pg.getByRole('table',{name:'空间成员',exact:true}).locator('tbody tr');
async function shot(pg,name){await pg.evaluate(()=>window.scrollTo(0,0));await pg.screenshot({path:fileURLToPath(new URL('screenshots/'+name+'.png',out)),fullPage:true})}
async function pass(name,fn){active=name;await fn();results.push({name,status:'PASS'});console.log('PASS '+name)}
async function facts(){const d={};for(const t of ['audit_event','execution','agent_task','agent_action_request','delivery_provider_attempt','outbox_event'])d[t]=(await db.query("SELECT count(*)::text AS count,md5(COALESCE(string_agg(row_to_json(t)::text,'|' ORDER BY id),'')) AS hash FROM "+t+' t')).rows[0];return d}
async function grant(enabled=true){await db.query('UPDATE platform_workspace_grant SET enabled=$3,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,a,enabled])}
const userRows=pg=>pg.getByRole('table',{name:'用户目录',exact:true}).locator('tbody tr'),account=pg=>pg.getByRole('region',{name:'用户详情',exact:true});
try{
 const before=await facts();
 await pass('actual product profile enables only explicit workspace account readers and no identity links',async()=>{
  const ledger=JSON.parse(readFileSync(new URL('processes.json',fixture),'utf8'));assert.ok(ledger.some(r=>r.name==='api-product'&&/api-product(?:\.exe)?$/.test(r.path)));
  const config=JSON.parse(readFileSync(new URL('product-profile.json',fixture),'utf8'));assert.deepEqual(config.directory.workspaces.map(w=>w.userProfiles),[true,true,false]);assert.equal(Object.hasOwn(config,'userProduct'),false);for(const w of config.directory.workspaces)assert.deepEqual(w.roles.find(r=>r.id==='audit_viewer').permissions,['management.audit.read']);
  const start=await control();assert.equal((await http.get(api+'/health/ready')).status(),200);const end=await control();for(const k of ['reads','orgReads','userReads'])assert.equal(end[k],start[k]);
  for(const [role,table,permission,want]of [['management_api','platform_workspace_grant','SELECT',true],['management_api','platform_workspace_grant','UPDATE',false],['management_api','audit_event','INSERT',false],['management_api','platform_browser_session','SELECT',false],['management_rp','platform_browser_session','INSERT',true],['management_rp','platform_workspace_grant','SELECT',false]])assert.equal((await db.query('SELECT has_table_privilege($1,$2,$3) AS allowed',[role,table,permission])).rows[0].allowed,want);
 });
 await pass('configuration declares profile capability without health or identity admission claims',async()=>{
  const start=await control(),t=await token();for(const ws of [a,b]){const r=await query(ws,'configuration',ws===a?t:await token('both'));assert.equal(r.data.queries.users,true);assert.equal(r.data.directory.availability,'NOT_CHECKED');assert.equal(r.writeEnabled,false)}const end=await control();assert.equal(end.userReads,start.userReads);assert.equal(end.reads,start.reads);
 });
 page=await person();
 await pass('whole product sidebar reaches all nine settings pages with honest configured and unavailable states',async()=>{
  const menu=page.getByRole('navigation',{name:'平台管理',exact:true});await expect(menu.getByRole('link')).toHaveCount(9);
  for(const [label,path,region]of [['设置总览','',null],['工作空间','workspaces','工作空间概览'],['组织机构','organizations','目录组织资料'],['空间成员','members','空间成员'],['用户目录','users','用户目录'],['角色权限','roles','角色权限'],['审计记录','audit','空间审计记录'],['运行日志','logs',null],['基础配置','configuration','基础配置概览']]){
   await menu.getByRole('link',{name:label,exact:true}).click();assert.equal(new URL(page.url()).pathname,'/settings'+(path?'/'+path:''));await expect(menu.getByRole('link',{name:label,exact:true})).toHaveAttribute('aria-current','page');
   if(region)await expect(page.getByRole(['members','users','roles','audit'].includes(path)?'table':'region',{name:region,exact:true})).toBeVisible();
   else await expect(page.getByRole('heading',{name:path==='logs'?'运行日志尚未接入':'设置总览',exact:true})).toBeVisible();
   assert.equal(await page.locator('main').getByRole('button',{name:/新建|邀请|保存|授权|删除|执行|发布/}).count(),0);
  }
  await shot(page,'whole-configuration');
 });
 for(const origin of ['configuration','workspace','organization'])await pass(origin+' full member account role and entry return preserve origin',async()=>{
  const title={configuration:'基础配置',workspace:'工作空间',organization:'组织资料'}[origin];
  await page.goto(p.origin+'/settings/'+{configuration:'configuration',workspace:'workspaces',organization:'organizations'}[origin]);
  if(origin==='configuration'){await page.getByRole('link',{name:'查看角色权限',exact:true}).click();await page.getByRole('table',{name:'角色权限'}).locator('tbody tr').filter({hasText:'管理查询员'}).getByRole('link',{name:'查看相关成员',exact:true}).click()}
  else await page.getByRole('region',{name:origin==='workspace'?'工作空间概览':'目录组织资料',exact:true}).getByRole('link',{name:origin==='workspace'?/^空间成员/:/^当前空间成员/}).click();
  await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('空间甲成员');await page.getByRole('combobox',{name:'角色',exact:true}).selectOption('management_viewer');await page.getByRole('combobox',{name:'状态',exact:true}).selectOption('active');await page.getByRole('button',{name:'筛选',exact:true}).click();await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(rows(page)).toHaveCount(7);
  await rows(page).last().getByRole('link',{name:'查看账号',exact:true}).click();await expect(account(page)).toBeVisible();
  await page.getByRole('link',{name:'查看角色权限',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('from'),origin);
  await page.getByRole('link',{name:'返回'+title,exact:true}).click();await expect(page.getByRole('region',{name:{configuration:'基础配置概览',workspace:'工作空间概览',organization:'目录组织资料'}[origin],exact:true})).toBeVisible();
 });
 await pass('actual product audit pages preserve exact records fixed configuration return and caller scope',async()=>{
  await page.goto(p.origin+'/settings/configuration');await page.getByRole('link',{name:'查看审计记录',exact:true}).click();const table=()=>page.getByRole('table',{name:'空间审计记录',exact:true});const ids=[];let copied;
  for(const n of [25,25,8]){await expect(table().locator('tbody tr')).toHaveCount(n);ids.push(...await table().locator('tbody details').allTextContents());if(n===25){await page.getByRole('link',{name:'更早记录',exact:true}).click();copied=page.url()}}
  assert.equal(new Set(ids).size,58);await reloadReadOnly(page);await expect(table().locator('tbody tr')).toHaveCount(8);await table().locator('summary').first().click();await shot(page,'whole-audit');await page.getByRole('link',{name:'返回基础配置',exact:true}).click();await expect(page.getByRole('region',{name:'基础配置概览'})).toBeVisible();
  const other=await person('双空间查询人员',b);await other.goto(copied);await expect(other.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible();assert.equal(await other.getByRole('table',{name:'空间审计记录'}).count(),0);
 });
 await pass('workspace without any source stays unconfigured in all actual product menus without native calls',async()=>{
  const d='14141414-1414-4414-8414-141414141414',t=await token('both'),start=await control(),config=await query(d,'configuration',t);
  assert.equal(config.data.directory.state,'UNCONFIGURED');assert.deepEqual(config.data.queries,{workspace:true,organization:false,members:false,users:false,roles:false,audit:false});const overview=await query(d,'workspace',t);assert.equal(overview.data.recordState,'UNCONFIGURED');
  for(const path of ['organization','members','users','roles','audit'])assert.equal((await query(d,path,t,503)).code,'SOURCE_UNCONFIGURED');assert.equal((await query(d,'runtime-logs/source',t)).data.state,'UNCONFIGURED');
  const other=await person('双空间查询人员',d);
  for(const path of ['members','users','roles','organizations','audit']){await other.goto(p.origin+'/settings/'+path);await expect(other.getByRole('heading',{name:'真实来源未配置',exact:true})).toBeVisible();assert.equal(await other.getByRole('table').count(),0)}
  await other.goto(p.origin+'/settings/logs');await expect(other.getByRole('heading',{name:'运行日志尚未接入',exact:true})).toBeVisible();await other.goto(p.origin+'/settings/workspaces');await expect(other.getByRole('region',{name:'工作空间概览'})).toContainText('尚未接入');await other.goto(p.origin+'/settings/configuration');await expect(other.getByRole('table',{name:'管理查询配置'}).getByRole('link')).toHaveCount(1);await shot(other,'whole-unconfigured');
  const end=await control();for(const k of ['reads','orgReads','userReads'])assert.equal(end[k],start[k]);
 });
 await pass('directory faults do not change configuration claims or independent audit and fixed error returns',async()=>{
  for(const [flag,path,message]of [['fail','members','成员目录暂时无法读取'],['fail','users','用户目录暂时无法读取'],['orgFail','organizations','组织资料暂时无法读取']]){
   try{await control({[flag]:true});await page.goto(p.origin+'/settings/configuration');await expect(page.getByRole('table',{name:'管理查询配置'})).toContainText('已配置');await page.goto(p.origin+'/settings/'+path+'?from=configuration');await expect(page.getByRole('heading',{name:message,exact:true})).toBeVisible();assert.equal(await page.getByRole('table').count(),0);await page.getByRole('link',{name:'返回基础配置',exact:true}).click();await page.getByRole('link',{name:'查看审计记录',exact:true}).click();await expect(page.getByRole('table',{name:'空间审计记录'}).locator('tbody tr')).toHaveCount(25)}finally{await control({[flag]:false})}
  }
 });
 await pass('management capability loss and token expiry deny every settings page without cached records',async()=>{
  const saved=(await db.query('SELECT capabilities FROM platform_workspace_grant WHERE actor_id=$1 AND workspace_id=$2',[actor,a])).rows[0];
  try{await login(page);await db.query("UPDATE platform_workspace_grant SET capabilities=ARRAY['BUSINESS_ACCEPTANCE_READ'],revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2",[actor,a]);for(const path of ['','workspaces','organizations','members','users','roles','audit','configuration','logs']){await page.goto(p.origin+'/settings'+(path?'/'+path:''));await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await page.getByRole('table').count(),0)}}
  finally{await db.query('UPDATE platform_workspace_grant SET capabilities=$3,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,a,saved.capabilities])}
  await login(page);await control({expireSubject:'admin-a'});for(const path of ['workspaces','organizations','members','users','roles','audit','configuration','logs']){await page.goto(p.origin+'/settings/'+path);await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await page.getByRole('table').count(),0)}await login(page);
 });
 await pass('authorized browser back and forward restore member list conditions and keyboard position',async()=>{
  await page.goto(p.origin+'/settings/members?offset=25&q='+encodeURIComponent('空间甲成员')+'&role=management_viewer&state=active&from=configuration');
  await expect(rows(page)).toHaveCount(7);const list=page.url(),row=rows(page).last(),id=await row.getAttribute('id');
  await row.getByRole('link',{name:'查看账号',exact:true}).click();await expect(account(page)).toBeVisible();
  await page.goBack();await expect(rows(page)).toHaveCount(7);
  assert.deepEqual([...new URL(page.url()).searchParams].sort(),[...new URL(list).searchParams].sort());
  await expect.poll(()=>page.locator('[id="'+id+'"]').evaluate(r=>r.contains(document.activeElement))).toBe(true);
  await page.goForward();await expect(account(page)).toBeVisible();
  await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeFocused();
 });
 await pass('history fragments never restore records or focus outside the fresh authorized list',async()=>{
  await page.goto(p.origin+'/settings/members?offset=25&role=management_viewer&state=active');await expect(rows(page)).toHaveCount(7);
  for(const id of ['member-not-visible','management-main','user-person-a-2']){
   await page.evaluate(id=>history.replaceState(history.state,'','#'+encodeURIComponent(id)),id);await reloadReadOnly(page);await expect(rows(page)).toHaveCount(7);
   assert.equal(await page.evaluate(()=>document.activeElement?.tagName==='TR'),false);
  }
  assert.equal(await page.evaluate(()=>localStorage.length+sessionStorage.length),0);
 });
 await pass('member account return preserves role search state second page row and refresh focus',async()=>{
  await page.goto(p.origin+'/settings/members?q='+encodeURIComponent('空间甲成员')+'&role=management_viewer&state=active&offset=25');await expect(rows(page)).toHaveCount(7);const list=page.url(),row=rows(page).last(),id=await row.getAttribute('id');await row.getByRole('link',{name:'查看账号',exact:true}).click();detailURL=page.url();await expect(account(page)).toBeVisible();await reloadReadOnly(page);await expect(account(page)).toBeVisible();await shot(page,'account-detail');await page.getByRole('link',{name:'返回成员列表',exact:true}).click();assert.deepEqual([...new URL(page.url()).searchParams].sort(),[...new URL(list).searchParams].sort());await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeFocused();await reloadReadOnly(page);await expect(page.locator('tr:target')).toBeFocused();await shot(page,'member-return');
 });
 await pass('member detail account membership loop retains original conditions and fixed origins',async()=>{
  for(const from of ['workspace','organization','configuration']){
   await page.goto(p.origin+'/settings/members?q='+encodeURIComponent('空间甲成员')+'&role=management_viewer&state=active&offset=25&from='+from);const id=await rows(page).first().getAttribute('id');await rows(page).first().getByRole('link',{name:'查看成员',exact:true}).click();const memberURL=page.url();await page.getByRole('link',{name:/^查看账号资料/}).click();await expect(account(page)).toBeVisible();await account(page).getByRole('link',{name:'查看成员',exact:true}).click();assert.equal(page.url(),memberURL);await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeFocused();assert.equal(new URL(page.url()).searchParams.get('from'),from);
  }
 });
 await pass('user directory pages deduplicate references and return restores search page and focused user',async()=>{
  await page.goto(p.origin+'/settings/users?q='+encodeURIComponent('空间甲成员'));const ids=[];for(const n of [25,25,8]){await expect(userRows(page)).toHaveCount(n);ids.push(...await userRows(page).evaluateAll(rs=>rs.map(r=>r.id)));if(n===25)await page.getByRole('link',{name:'下一页',exact:true}).click()}assert.equal(new Set(ids).size,58);
  const list=page.url(),id=await userRows(page).last().getAttribute('id');await userRows(page).last().getByRole('link',{name:'查看用户',exact:true}).click();await expect(account(page)).toBeVisible();await page.getByRole('link',{name:'返回用户目录',exact:true}).click();assert.deepEqual([...new URL(page.url()).searchParams].sort(),[...new URL(list).searchParams].sort());await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeFocused();await reloadReadOnly(page);await expect(page.locator('tr:target')).toBeFocused();await shot(page,'user-return');
 });
 await pass('account membership state and canonical identity stay separate despite a seeded same-person candidate',async()=>{
  const t=await token();for(const [id,accountState,memberState]of [['person-a-2','active','active'],['person-a-5','inactive','active'],['person-a-7','active','inactive']]){const d=(await query(a,'users?userId='+id,t)).data.selected;assert.equal(d.profile.accountState,accountState);assert.equal(d.memberships[0].state,memberState);assert.deepEqual(d.identity,{workspaceId:a,state:'UNCONFIGURED',actorId:null,revision:'',basisRef:'',expiresAt:null})}
  await page.goto(p.origin+'/settings/users?userId=person-a-2');await expect(account(page)).toContainText('关联来源未配置');await shot(page,'identity-unconfigured');
 });
 await pass('per-workspace disabled profile refuses lists and details before any source call',async()=>{
  const t=await token('both'),start=await control(),config=await query(c,'configuration',t);assert.equal(config.data.directory.state,'CONFIGURED');assert.equal(config.data.queries.members,true);assert.equal(config.data.queries.users,false);for(const path of ['users','users?userId=person-b-2'])assert.equal((await query(c,path,t,503)).code,'SOURCE_UNCONFIGURED');const end=await control();assert.equal(end.userReads,start.userReads);assert.equal(end.reads,start.reads);
  const other=await person('双空间查询人员',c);await other.goto(p.origin+'/settings/members');await expect(rows(other)).toHaveCount(17);await rows(other).first().getByRole('link',{name:'查看账号',exact:true}).click();await expect(other.getByRole('heading',{name:'真实来源未配置',exact:true})).toBeVisible();await other.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(other.locator('tr:target')).toBeFocused();
 });
 await pass('copied account IDs and member navigation cannot cross workspace or invent membership associations',async()=>{
  const t=await token('both'),start=await control();assert.equal((await query(b,'users?userId=person-a-2',t,404)).code,'USER_UNAVAILABLE');assert.equal((await control()).userReads,start.userReads);
  const other=await person('双空间查询人员',b);await other.goto(detailURL);await expect(other.getByRole('heading',{name:'指定用户不可见',exact:true})).toBeVisible();assert.equal(await account(other).count(),0);await other.getByRole('link',{name:'返回成员列表',exact:true}).click();assert.equal(await other.locator('tr:target').count(),0);
  await page.goto(p.origin+'/settings/users?userId=person-a-2&memberContext='+encodeURIComponent('memberId=grant-a-003'));await expect(page.getByRole('heading',{name:'当前记录无法读取',exact:true})).toBeVisible();assert.equal(await account(page).count(),0);
 });
 await pass('navigation is bounded UI context never API authority or an arbitrary return URL',async()=>{
  const start=await control();for(const context of ['memberId=grant-a-002&memberId=grant-a-003','memberId=grant-a-002&workspaceId='+b,'memberId=grant-a-002&returnTo=https://external.invalid','memberId=grant-a-002&memberContext=nested']){await page.goto(p.origin+'/settings/users?userId=person-a-2&memberContext='+encodeURIComponent(context));await expect(page.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible()}assert.equal((await control()).userReads,start.userReads);
  const t=await token();for(const tail of ['&memberContext=memberId%3Dgrant-a-002','&workspaceId='+b,'&organizationId=organization-b','&userId=person-a-3'])await query(a,'users?userId=person-a-2'+tail,t,400);
 });
 await pass('native user permissions foreign ownership unknown state and failures expose no profiles and recover explicitly',async()=>{
  for(const [flag,value]of [['userPermission',false],['userForeign',true],['userUnknown',true],['userFail',true]]){
   try{await control({[flag]:value});await page.goto(detailURL);await expect(page.getByRole('heading',{name:'用户目录暂时无法读取',exact:true})).toBeVisible();assert.equal(await account(page).count(),0);await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(rows(page)).toHaveCount(7)}finally{await control({[flag]:!value})}
  }await page.goto(detailURL);await expect(account(page)).toBeVisible();
 });
 for(const source of ['success','failure'])for(const mode of ['revoke','expire','grant','capability','grant-expiry'])await pass('in-flight account lookup rechecks caller '+mode+' with source '+source,async()=>{
  const t=await token(),saved=(await db.query('SELECT enabled,capabilities,valid_until FROM platform_workspace_grant WHERE actor_id=$1 AND workspace_id=$2',[actor,a])).rows[0];let pending;
  try{await control({holdUser:true});pending=http.get(prefix(a)+'users?userId=person-a-2',{headers:{Authorization:'Bearer '+t},maxRedirects:0});await expect.poll(async()=>(await control()).userWaiting).toBe(1);if(mode==='grant')await grant(false);if(mode==='capability')await db.query("UPDATE platform_workspace_grant SET capabilities=ARRAY['BUSINESS_ACCEPTANCE_READ'],revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2",[actor,a]);if(mode==='grant-expiry')await db.query("UPDATE platform_workspace_grant SET valid_until=now()-interval '1 second',revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2",[actor,a]);await control({userPermission:source==='success',holdUser:false,...mode==='revoke'?{revokeSubject:'admin-a'}:mode==='expire'?{expireSubject:'admin-a'}:{}});const r=await pending;assert.equal(r.status(),mode==='capability'?403:401);assert.equal(Object.hasOwn(await r.json(),'data'),false)}
  finally{await control({userPermission:true,holdUser:false});await pending?.catch(()=>{});if(['grant','capability','grant-expiry'].includes(mode))await db.query('UPDATE platform_workspace_grant SET enabled=$3,capabilities=$4,valid_until=$5,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,a,saved.enabled,saved.capabilities,saved.valid_until])}
 });
 await pass('membership removed during account lookup denies a formerly visible profile',async()=>{
  const t=await token();let pending;try{await control({holdUser:true});pending=query(a,'users?userId=person-a-2',t,404);await expect.poll(async()=>(await control()).userWaiting).toBe(1);await control({removeUser:true,holdUser:false});assert.equal((await pending).code,'USER_UNAVAILABLE')}finally{await control({removeUser:false,holdUser:false});await pending?.catch(()=>{})}
 });
 await pass('native permission lost during profile lookup never returns stale account data',async()=>{
  const t=await token();let pending;try{await control({holdUser:true});pending=query(a,'users?userId=person-a-2',t,503);await expect.poll(async()=>(await control()).userWaiting).toBe(1);await control({userPermission:false,holdUser:false});assert.equal((await pending).code,'USER_DIRECTORY_UNAVAILABLE')}finally{await control({userPermission:true,holdUser:false});await pending?.catch(()=>{})}
 });
 await pass('revocation expiry refresh and browser history cannot revive account records or previous RP revisions',async()=>{
  await login(page);await page.goto(p.origin+'/settings/members');await rows(page).first().getByRole('link',{name:'查看账号',exact:true}).click();await expect(account(page)).toBeVisible();try{await grant(false);await reloadReadOnly(page);await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();await page.goBack();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await account(page).count(),0)}finally{await grant(true)}
  await page.goto(detailURL);await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();await login(page);await page.goto(detailURL);await expect(account(page)).toBeVisible();await control({expireSubject:'admin-a'});await reloadReadOnly(page);await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await account(page).count(),0);await login(page);
 });
 for(const mode of ['revoke','expire'])await pass('web account response held in source I/O refuses '+mode+' before rendering data',async()=>{
  await login(page);let pending;
  try{await control({holdUser:true});pending=page.goto(detailURL);await expect.poll(async()=>(await control()).userWaiting).toBe(1);await control({holdUser:false,...mode==='revoke'?{revokeSubject:'admin-a'}:{expireSubject:'admin-a'}});await pending;await expect(page.getByRole('heading',{name:'当前身份或记录无法核验',exact:true})).toBeVisible();assert.equal(await account(page).count(),0)}
  finally{await control({holdUser:false});await pending?.catch(()=>{})}
  await login(page);await page.goto(detailURL);await expect(account(page)).toBeVisible();
 });
 await pass('product has no account writes native proxy model or business command routes',async()=>{
  const t=await token();for(const method of ['POST','PATCH','DELETE'])assert.equal((await http.fetch(prefix(a)+'users',{method,headers:{Authorization:'Bearer '+t},data:{actorId:actor}})).status(),405);for(const path of ['/api/agent-tasks','/api/v1/executions','/v2/users/person-a-2','/api/v1/workspaces/'+a+'/management/users/person-a-2'])assert.equal((await http.get(api+path,{headers:{Authorization:'Bearer '+t}})).status(),404);await query(a,'users',await token('ordinary'),403);await query(b,'users',await token(),401);
 });
 await pass('mobile account detail uses business language and keeps credentials private with business facts unchanged',async()=>{
  await page.setViewportSize({width:390,height:844});await page.goto(detailURL);await expect(account(page)).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await shot(page,'mobile');assert.equal(await page.evaluate(()=>localStorage.length+sessionStorage.length),0);const html=await page.content();for(const key of ['pgPassword','apiPassword','rpPassword','rpSecret','resourceSecret','readCredential','control','encryptionKey','adminDSN'])assert.ok(!html.includes(p[key]));for(const privateField of ['PRIVATE_USER_EMAIL','PRIVATE_USER_PHONE'])assert.ok(!html.includes(privateField));assert.deepEqual(await facts(),before);assert.deepEqual(errors,[]);assert.deepEqual(writes,[]);assert.deepEqual(external,[]);
 });
}catch(e){let error=e.message;for(const v of Object.values(p))if(typeof v==='string'&&v.length>8)error=error.split(v).join('[redacted]');error=error.replace(/Bearer\s+[^\s]+/gi,'Bearer [redacted]');results.push({name:active,status:'FAIL',error});console.error('FAIL '+active+': '+error);process.exitCode=1;try{if(page)await shot(page,'failure')}catch{}}
finally{await control({holdUser:false,holdList:false,userPermission:true,userForeign:false,userFail:false,userUnknown:false,removeUser:false}).catch(()=>{});writeFileSync(new URL('browser-results.json',out),JSON.stringify({entrypoint:'cmd/api',channel,results,errors,writes,external,environment:'ISOLATED_SYNTHETIC',realSources:'UNCONFIGURED',identityMapping:'UNCONFIGURED',businessWrites:false,models:false,dispatch:false},null,2));for(const c of contexts)await c.close();await http.dispose();await db.end();await browser.close()}
