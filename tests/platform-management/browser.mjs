// Isolated Edge/CI Chromium: real RP SDK + canonical PG bindings + synthetic native directory.
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
const root=fileURLToPath(new URL('../../',import.meta.url)),fixtureRun=new URL('../../.data/management-readonly-20261025/',import.meta.url);
const output=process.env.MANAGEMENT_BROWSER_OUTPUT??'management-readonly-20261025';if(!/^management-readonly-[a-z0-9-]+$/.test(output))throw Error('independent evidence directory required');
const run=new URL('../../.data/'+output+'/',import.meta.url);
const require=createRequire(new URL('../browser/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test'),{Pool}=createRequire(new URL('../../apps/web/package.json',import.meta.url))('pg');
const p=JSON.parse(readFileSync(new URL('profile.json',fixtureRun),'utf8'));assert.equal(p.origin,'https://localhost:4300');
const db=new Pool({connectionString:p.adminDSN,ssl:{ca:readFileSync(p.cert),rejectUnauthorized:true}}),results=[],contexts=[],errors=[],writes=[],external=[];
const channel=process.env.MANAGEMENT_BROWSER_CHANNEL??'msedge';if(!['msedge','chromium'].includes(channel))throw Error('explicit browser required');
const browser=await chromium.launch({...(channel==='msedge'?{channel}:{}),headless:true}),a='11111111-1111-4111-8111-111111111111',b='12121212-1212-4212-8212-121212121212';
mkdirSync(new URL('screenshots',run),{recursive:true});
async function person(label,ws=a){
 const context=await browser.newContext({ignoreHTTPSErrors:true,serviceWorkers:'block',viewport:{width:1600,height:950}});contexts.push(context);
 await context.route('**/*',route=>{const r=route.request(),u=new URL(r.url());if(![p.origin,p.issuer].includes(u.origin)){external.push(u.origin);return route.abort()}if(u.origin===p.origin&&!['GET','HEAD'].includes(r.method())&&!u.pathname.startsWith('/auth/')){writes.push(u.pathname);return route.abort()}return route.continue()});
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await login(page,label,ws);return {page,context};
}
async function login(page,label,ws=a){await page.goto(p.origin+'/auth/login?workspaceId='+ws);await page.getByRole('button',{name:'使用统一身份登录',exact:true}).click();await page.getByRole('button',{name:label,exact:true}).click();await page.waitForURL(p.origin+'/**');}
async function control(data){const response=await browser.newContext({ignoreHTTPSErrors:true});try{const r=await response.request.post(p.issuer+'/_control',{headers:{'x-fixture-control':p.control},data});assert.equal(r.status(),200);return await r.json()}finally{await response.close()}}
const rows=page=>page.getByRole('table',{name:'空间成员',exact:true}).locator('tbody tr');
async function businessHash(){return (await db.query("SELECT md5(string_agg(row_to_json(t)::text,'|' ORDER BY id)) AS hash FROM audit_event t")).rows[0].hash}
async function pass(name,fn){active=name;await fn();results.push({name,status:'PASS'});console.log('PASS '+name)}
let active='setup';
try{
 const before=await businessHash();
 await pass('minimal DB roles: API SELECT only, RP temporary sessions only',async()=>{
  for(const [role,table,permission,expected] of [['management_api','audit_event','SELECT',true],['management_api','audit_event','UPDATE',false],['management_api','platform_workspace_grant','UPDATE',false],['management_api','platform_browser_session','SELECT',false],['management_rp','platform_browser_session','INSERT',true],['management_rp','platform_workspace_grant','SELECT',false]])assert.equal((await db.query('SELECT has_table_privilege($1,$2,$3) AS allowed',[role,table,permission])).rows[0].allowed,expected);
  const flags=(await db.query("SELECT rolname,rolsuper,rolcreatedb,rolcreaterole,rolreplication,rolbypassrls FROM pg_roles WHERE rolname=ANY($1)",[['management_api','management_rp']])).rows;assert.equal(flags.length,2);for(const flag of flags)for(const [k,v]of Object.entries(flag))if(k!=='rolname')assert.equal(v,false);
 });
 active='login';const one=await person('查询人员 A'),page=one.page;let roleListURL,roleDetailURL;
 await pass('canonical individual RP session and modern management shell',async()=>{await expect(page.getByRole('heading',{name:'设置总览',exact:true})).toBeVisible();await expect.poll(()=>page.locator('.brand img').evaluate(el=>el.complete&&el.naturalWidth>0)).toBe(true);await page.getByRole('link',{name:'身份与工作空间',exact:true}).click();await expect(page.getByTestId('management-principal')).toHaveText('83000000-0000-4000-8000-000000000001');await expect(page.getByTestId('management-workspace')).toHaveText(a);const cookie=(await one.context.cookies(p.origin)).find(v=>v.name==='__Host-dpp-session');assert.ok(cookie.secure&&cookie.httpOnly&&cookie.sameSite==='Lax');});
 await pass('member pagination covers 58 distinct records and exact detail survives reload',async()=>{
  await page.goto(p.origin+'/settings/members');await expect(rows(page)).toHaveCount(25);const first=await rows(page).allTextContents();await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(rows(page)).toHaveCount(25);assert.notDeepEqual(await rows(page).allTextContents(),first);await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(rows(page)).toHaveCount(8);await rows(page).last().getByRole('link',{name:'查看成员',exact:true}).click();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('空间甲成员 057');await page.reload();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('空间甲成员 057');await page.screenshot({path:fileURLToPath(new URL('screenshots/members.png',run)),fullPage:true});
 });
 await pass('filter searches entire directory before paging; empty and unavailable stay distinct',async()=>{
  await page.goto(p.origin+'/settings/members');await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('搜索目标');await page.getByRole('button',{name:'筛选',exact:true}).click();await expect(rows(page)).toHaveCount(1);await expect(rows(page)).toContainText('057');await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('无匹配');await page.getByRole('button',{name:'筛选',exact:true}).click();await expect(rows(page)).toHaveCount(0);await expect(page.getByText('当前条件下没有成员',{exact:true})).toBeVisible();await page.getByRole('link',{name:'清除条件',exact:true}).click();await expect(rows(page)).toHaveCount(25);
 });
 await pass('role projection uses explicit readonly application catalog with no management writes',async()=>{
  await page.goto(p.origin+'/settings/roles');await expect(page.getByRole('table',{name:'角色权限',exact:true})).toContainText('管理查询员');await expect(page.getByRole('table',{name:'角色权限',exact:true})).toContainText('审计观察员');assert.equal(await page.getByRole('button',{name:/保存|新增|删除|授权/}).count(),0);await page.screenshot({path:fileURLToPath(new URL('screenshots/roles.png',run)),fullPage:true});
 });
 await pass('role to members to exact detail restores filters page and focused row after refresh',async()=>{
  await page.getByRole('table',{name:'角色权限',exact:true}).getByRole('row').filter({hasText:'管理查询员'}).getByRole('link',{name:'查看相关成员',exact:true}).click();
  await expect(rows(page)).toHaveCount(25);await expect(page.getByRole('combobox',{name:'角色',exact:true})).toHaveValue('management_viewer');
  await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('空间甲成员');await page.getByRole('combobox',{name:'状态',exact:true}).selectOption('active');await page.getByRole('button',{name:'筛选',exact:true}).click();
  await expect(rows(page)).toHaveCount(25);await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(rows(page)).toHaveCount(7);
  roleListURL=page.url();const ids=await rows(page).evaluateAll(elements=>elements.map(element=>element.id));
  await page.screenshot({path:fileURLToPath(new URL('screenshots/role-members-list.png',run)),fullPage:true});
  await rows(page).last().getByRole('link',{name:'查看成员',exact:true}).click();roleDetailURL=page.url();
  await expect(page.getByRole('heading',{name:'成员详情',exact:true})).toBeVisible();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('055');assert.equal(await rows(page).count(),0);
  await page.reload();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('055');await page.screenshot({path:fileURLToPath(new URL('screenshots/role-member-detail.png',run)),fullPage:true});
  await page.goBack();await expect(rows(page)).toHaveCount(7);assert.equal(new URL(page.url()).search,new URL(roleListURL).search);await page.goForward();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('055');
  await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(rows(page)).toHaveCount(7);
  const restored=new URL(page.url());assert.equal(restored.search,new URL(roleListURL).search);assert.equal(restored.hash,'#member-grant-a-055');
  await expect(page.locator('tr:target')).toHaveAttribute('id','member-grant-a-055');await expect(page.locator('tr:target')).toBeInViewport();
  await page.reload();await expect(rows(page)).toHaveCount(7);assert.deepEqual(await rows(page).evaluateAll(elements=>elements.map(element=>element.id)),ids);
  await expect(page.getByRole('textbox',{name:'搜索成员',exact:true})).toHaveValue('空间甲成员');await expect(page.getByRole('combobox',{name:'角色',exact:true})).toHaveValue('management_viewer');await expect(page.getByRole('combobox',{name:'状态',exact:true})).toHaveValue('active');
  await page.evaluate(()=>window.scrollTo(0,0));await page.screenshot({path:fileURLToPath(new URL('screenshots/role-members-return.png',run)),fullPage:true});
 });
 await pass('changing role filters resets the page while preserving the other selected conditions',async()=>{
  await page.getByRole('combobox',{name:'角色',exact:true}).selectOption('audit_viewer');await page.getByRole('button',{name:'筛选',exact:true}).click();await expect(rows(page)).toHaveCount(17);
  const selected=new URL(page.url());assert.equal(Number(selected.searchParams.get('offset')??0),0);assert.equal(selected.searchParams.has('memberId'),false);
  await expect(page.getByRole('textbox',{name:'搜索成员',exact:true})).toHaveValue('空间甲成员');await expect(page.getByRole('combobox',{name:'状态',exact:true})).toHaveValue('active');
  await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('无匹配');await page.getByRole('button',{name:'筛选',exact:true}).click();await expect(rows(page)).toHaveCount(0);await expect(page.getByText('当前条件下没有成员',{exact:true})).toBeVisible();
  await page.getByRole('link',{name:'清除条件',exact:true}).click();await expect(rows(page)).toHaveCount(25);await expect(page.getByRole('combobox',{name:'角色',exact:true})).toHaveValue('');
 });
 await pass('unavailable detail and out-of-range page return explicitly without losing original filters',async()=>{
  const missing=new URL(roleDetailURL);missing.searchParams.set('memberId','grant-a-missing');await page.goto(missing.href);await expect(page.getByRole('heading',{name:'指定成员不可见',exact:true})).toBeVisible();assert.equal(await page.getByRole('region',{name:'成员详情',exact:true}).count(),0);
  await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(rows(page)).toHaveCount(7);assert.equal(new URL(page.url()).search,new URL(roleListURL).search);
  const beyond=new URL(roleListURL);beyond.searchParams.set('offset','100');await page.goto(beyond.href);await expect(rows(page)).toHaveCount(0);assert.equal(new URL(page.url()).searchParams.get('offset'),'100');
  await page.getByRole('link',{name:'保留筛选，返回首页',exact:true}).click();await expect(rows(page)).toHaveCount(25);await expect(page.getByRole('combobox',{name:'角色',exact:true})).toHaveValue('management_viewer');await expect(page.getByRole('combobox',{name:'状态',exact:true})).toHaveValue('active');
 });
 let auditCursor;
 await pass('audit immutable references paginate 25/25/8 without raw payloads',async()=>{
  await page.goto(p.origin+'/settings/audit');const table=()=>page.getByRole('table',{name:'空间审计记录',exact:true});await expect(table().locator('tbody tr')).toHaveCount(25);auditCursor=new URL(await page.getByRole('link',{name:'更早记录',exact:true}).getAttribute('href'),p.origin).searchParams.get('cursor');const ids=new Set(await table().locator('tbody details').allTextContents());await table().locator('summary').first().click();assert.ok(!(await page.content()).includes('NEVER_RENDER_RAW_PAYLOAD'));await page.screenshot({path:fileURLToPath(new URL('screenshots/audit.png',run)),fullPage:true});await page.getByRole('link',{name:'更早记录',exact:true}).click();await expect(table().locator('tbody tr')).toHaveCount(25);for(const text of await table().locator('tbody details').allTextContents()){assert.ok(!ids.has(text));ids.add(text)}await page.getByRole('link',{name:'更早记录',exact:true}).click();await expect(table().locator('tbody tr')).toHaveCount(8);for(const text of await table().locator('tbody details').allTextContents()){assert.ok(!ids.has(text));ids.add(text)}assert.equal(ids.size,58);
 });
 await pass('foreign member, duplicate query and unknown workspace selector fail without fallback',async()=>{
  for(const query of ['?memberId=grant-b-000','?offset=0&offset=25','?workspaceId='+b]){await page.goto(p.origin+'/settings/members'+query);assert.equal(await rows(page).count(),0);await expect(page.getByRole('heading',{name:/指定成员不可见|查询条件无效/})).toBeVisible()}
 });
 await pass('workspace without grant and ordinary acceptance-only user cannot enter management',async()=>{
  await login(page,'查询人员 A',b);await expect(page.getByRole('heading',{name:'登录未完成',exact:true})).toBeVisible();const ordinary=await person('无管理权限人员');await expect(ordinary.page.getByRole('heading',{name:'登录未完成',exact:true})).toBeVisible();
 });
 const both=await person('双空间查询人员'),two=both.page;
 await pass('role member navigation is reevaluated in the current workspace and copied foreign detail is refused',async()=>{
  await two.goto(roleDetailURL);await expect(two.getByRole('region',{name:'成员详情',exact:true})).toContainText('空间甲成员');
  await two.getByRole('link',{name:'身份与工作空间',exact:true}).click();await two.getByRole('link',{name:'管理工作空间 B',exact:true}).click();await two.getByRole('button',{name:'使用统一身份登录',exact:true}).click();await two.getByRole('button',{name:'双空间查询人员',exact:true}).click();await two.waitForURL(p.origin+'/**');
  assert.equal(new URL(two.url()).search,'');await two.goto(p.origin+'/settings/roles');await two.getByRole('table',{name:'角色权限',exact:true}).getByRole('row').filter({hasText:'管理查询员'}).getByRole('link',{name:'查看相关成员',exact:true}).click();await expect(rows(two)).toHaveCount(11);assert.ok(!(await two.textContent('body')).includes('空间甲成员'));
  await two.goto(roleDetailURL);await expect(two.getByRole('heading',{name:'指定成员不可见',exact:true})).toBeVisible();assert.equal(await two.getByRole('region',{name:'成员详情',exact:true}).count(),0);assert.equal(await rows(two).count(),0);
  await login(two,'双空间查询人员',a);
 });
 await pass('switching authorized workspaces clears old members and rejects caller/workspace cursors',async()=>{
  await two.goto(p.origin+'/settings/audit?cursor='+auditCursor);await expect(two.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible();await two.goto(p.origin+'/settings/audit');const cursorB=new URL(await two.getByRole('link',{name:'更早记录',exact:true}).getAttribute('href'),p.origin).searchParams.get('cursor');await two.getByRole('link',{name:'身份与工作空间',exact:true}).click();await two.getByRole('link',{name:'管理工作空间 B',exact:true}).click();await two.getByRole('button',{name:'使用统一身份登录',exact:true}).click();await two.getByRole('button',{name:'双空间查询人员',exact:true}).click();await two.waitForURL(p.origin+'/**');await two.goto(p.origin+'/settings/members');await expect(rows(two)).toHaveCount(17);assert.ok(!(await two.textContent('body')).includes('空间甲成员'));await expect(rows(two).first()).toContainText('空间乙成员');await two.goto(p.origin+'/settings/audit?cursor='+cursorB);await expect(two.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible();
 });
 await pass('revocation removes reads and restoration cannot resurrect old RP revision',async()=>{
  await login(page,'查询人员 A');await page.goto(roleListURL);await expect(rows(page)).toHaveCount(7);await rows(page).last().getByRole('link',{name:'查看成员',exact:true}).click();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('055');
  await db.query('UPDATE platform_workspace_grant SET enabled=false,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',['83000000-0000-4000-8000-000000000001',a]);
  await page.goBack();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);await page.goForward();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await page.getByRole('region',{name:'成员详情',exact:true}).count(),0);
  await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);
  await db.query('UPDATE platform_workspace_grant SET enabled=true,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',['83000000-0000-4000-8000-000000000001',a]);
  for(const target of [roleListURL,roleDetailURL,p.origin+'/settings/roles',p.origin+'/settings/audit']){await page.goto(target);await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await page.getByRole('region',{name:'成员详情',exact:true}).count(),0)}
  await login(page,'查询人员 A');await page.goto(roleDetailURL);await expect(page.getByRole('region',{name:'成员详情',exact:true})).toContainText('055');await page.getByRole('link',{name:'返回成员列表',exact:true}).click();await expect(rows(page)).toHaveCount(7);assert.equal(new URL(page.url()).search,new URL(roleListURL).search);
  await page.goto(p.origin+'/settings/members');await expect(rows(page)).toHaveCount(25);
 });
 await pass('native directory permission revocation and foreign responses discard data',async()=>{
  await control({permission:false});await page.reload();await expect(page.getByRole('heading',{name:'成员目录暂时无法读取',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);assert.equal(await page.getByText('当前条件下没有成员',{exact:true}).count(),0);
  await control({permission:true,foreign:true});await page.reload();await expect(page.getByRole('heading',{name:'成员目录暂时无法读取',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);await control({foreign:false});await page.reload();await expect(rows(page)).toHaveCount(25);
 });
 await pass('direct API discards directory records when access token is revoked during the read',async()=>{
  const direct=await browser.newContext({ignoreHTTPSErrors:true});
  const issued=await control({issueSubject:'admin-a',holdList:true});
  let pending;
  try{
   pending=direct.request.get('https://localhost:18499/api/v1/workspaces/'+a+'/management/members',{headers:{Authorization:'Bearer '+issued.testAccessToken}});
   await expect.poll(async()=>(await control({})).waiting,{timeout:3000}).toBe(1);
   await control({revokeSubject:'admin-a',holdList:false});
   const response=await pending;assert.equal(response.status(),401);const denied=await response.json();assert.equal(denied.code,'AUTHENTICATION_REQUIRED');assert.equal(Object.hasOwn(denied,'data'),false);
   await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);
   await login(page,'查询人员 A');await page.goto(p.origin+'/settings/members');await expect(rows(page)).toHaveCount(25);
  }finally{await control({holdList:false});if(pending)await pending.catch(()=>{});await direct.close()}
 });
 await pass('management profile has no business, model, Agent or write routes',async()=>{
  for(const path of ['/settings/members','/api/agent-tasks','/api/v1/business-acceptances','/core/acceptance-context'])assert.equal((await one.context.request.post(p.origin+path,{data:{workspaceId:a}})).status(),403);
  assert.equal((await one.context.request.get(p.origin+'/agent')).status(),403);assert.equal((await one.context.request.get('https://localhost:18499/api/v1/workspaces/'+a+'/management/members')).status(),400);
 });
 await pass('mobile management and read-only history remain intact after queries',async()=>{
  await page.setViewportSize({width:390,height:844});await page.goto(p.origin+'/settings/members');await expect(rows(page)).toHaveCount(25);assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:fileURLToPath(new URL('screenshots/mobile.png',run)),fullPage:true});assert.equal(await businessHash(),before);for(const table of ['execution','agent_task','agent_action_request','delivery_provider_attempt','outbox_event'])assert.equal(Number((await db.query('SELECT count(*) AS n FROM '+table)).rows[0].n),0);
 });
 assert.deepEqual(errors,[]);assert.deepEqual(writes,[]);assert.deepEqual(external,[]);
}catch(error){process.exitCode=1;results.push({name:'failure near '+active,status:'FAIL',error:String(error).slice(0,1300)});console.log('FAIL isolated management browser; non-secret failure details in result file');}
finally{await control({permission:true,foreign:false,fail:false,holdList:false}).catch(()=>{});writeFileSync(new URL('browser-results.json',run),JSON.stringify({recordedAt:new Date().toISOString(),browser:channel,source:'SYNTHETIC_FIXTURE',realSources:'UNCONFIGURED',businessWrites:false,models:false,dispatch:false,errors,writes,external,verdict:process.exitCode?'FAIL':'PASS',results},null,2));await browser.close();await db.end()}
