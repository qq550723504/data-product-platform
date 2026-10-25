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
 const userRows=()=>page.getByRole('table',{name:'用户目录',exact:true}).locator('tbody tr'),userDetail=()=>page.getByRole('region',{name:'用户详情',exact:true});let userListURL,userURL;
 await pass('scoped user pages deduplicate references and restore search page and selected row',async()=>{
  await page.goto(p.origin+'/settings/users');await expect(userRows()).toHaveCount(25);const first=await userRows().allTextContents();await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(userRows()).toHaveCount(25);assert.notDeepEqual(await userRows().allTextContents(),first);await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(userRows()).toHaveCount(8);
  await page.goto(p.origin+'/settings/users?offset=25&q='+encodeURIComponent('空间甲成员'));await expect(userRows()).toHaveCount(25);userListURL=page.url();const id=await userRows().last().getAttribute('id');await userRows().last().getByRole('link',{name:'查看用户',exact:true}).click();userURL=page.url();await expect(userDetail()).toBeVisible();await page.reload();await expect(userDetail()).toBeVisible();await page.getByRole('link',{name:'返回用户目录',exact:true}).click();assert.equal(new URL(page.url()).search,new URL(userListURL).search);await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeInViewport();await page.evaluate(()=>scrollTo(0,0));await page.screenshot({path:fileURLToPath(new URL('screenshots/users-return.png',run)),fullPage:true});
  await page.getByRole('textbox',{name:'搜索用户',exact:true}).fill('搜索目标');await page.getByRole('button',{name:'查询',exact:true}).click();await expect(userRows()).toHaveCount(1);await expect(userRows()).toContainText('057');assert.equal(new URL(page.url()).searchParams.has('offset'),false);await page.getByRole('link',{name:'清除条件',exact:true}).click();await expect(userRows()).toHaveCount(25);
  await page.screenshot({path:fileURLToPath(new URL('screenshots/users.png',run)),fullPage:true});
 });
 await pass('account status is separate from membership and canonical identity is explicit only',async()=>{
  await page.goto(p.origin+'/settings/users?userId=person-a-7');await expect(userDetail().locator('dd').nth(1)).toHaveText('启用');await expect(page.getByRole('table',{name:'用户的空间成员关系'}).locator('tbody tr td').first()).toHaveText('停用');await expect(userDetail().locator('dd').nth(2)).toHaveText('未关联');
  await page.goto(p.origin+'/settings/users?userId=person-a-5');await expect(userDetail().locator('dd').nth(1)).toHaveText('停用');await expect(page.getByRole('table',{name:'用户的空间成员关系'}).locator('tbody tr td').first()).toHaveText('启用');
  await page.goto(p.origin+'/settings/users?userId=person-a-2');await expect(userDetail().locator('dd').nth(2)).toHaveText('已关联');await userDetail().getByText('查看资料与身份关联依据',{exact:true}).click();await expect(userDetail()).toContainText('84000000-0000-4000-8000-000000000002');assert.ok(!(await userDetail().textContent()).includes('83000000-0000-4000-8000-000000000001'));assert.ok(!(await page.textContent('body')).includes('PRIVATE_USER_'));await page.screenshot({path:fileURLToPath(new URL('screenshots/user-detail.png',run)),fullPage:true});
 });
 await pass('user to member and role queries return to exact user details and original list',async()=>{
  await page.goto(userURL);await userDetail().getByRole('link',{name:'查看成员',exact:true}).click();await expect(page.getByRole('region',{name:'成员详情',exact:true})).toBeVisible();await page.goBack();await expect(userDetail()).toBeVisible();assert.equal(page.url(),userURL);await userDetail().getByRole('link',{name:'查看角色权限',exact:true}).click();await expect(page.getByRole('table',{name:'角色权限'})).toBeVisible();await page.goBack();await expect(userDetail()).toBeVisible();await page.getByRole('link',{name:'返回用户目录',exact:true}).click();assert.equal(new URL(page.url()).search,new URL(userListURL).search);
 });
 await pass('user missing foreign owner unknown state and revoked native user-read discard records',async()=>{
  const reads=(await control({})).userReads;await page.goto(p.origin+'/settings/users?userId=person-b-2');await expect(page.getByRole('heading',{name:'指定用户不可见',exact:true})).toBeVisible();assert.equal((await control({})).userReads,reads);
  for(const fault of ['userPermission','userForeign','userFail','userUnknown']){await control({[fault]:fault!=='userPermission'});await page.goto(p.origin+'/settings/users?userId=person-a-2');await expect(page.getByRole('heading',{name:'用户目录暂时无法读取',exact:true})).toBeVisible();assert.equal(await userDetail().count(),0);await control({[fault]:fault==='userPermission'})}
  await page.goto(p.origin+'/settings/users?offset=100&q='+encodeURIComponent('空间甲成员'));await expect(userRows()).toHaveCount(0);await page.getByRole('link',{name:'保留条件，返回首页',exact:true}).click();await expect(userRows()).toHaveCount(25);await expect(page.getByRole('textbox',{name:'搜索用户',exact:true})).toHaveValue('空间甲成员');
 });
 await pass('user association is reevaluated on selected actor revocation without granting caller access',async()=>{
  const selected='84000000-0000-4000-8000-000000000002';
  try{await db.query('UPDATE platform_actor_state SET enabled=false,revision=revision+1 WHERE actor_id=$1',[selected]);await page.goto(p.origin+'/settings/users?userId=person-a-2');await expect(userDetail().locator('dd').nth(2)).toHaveText('未关联');assert.ok(!(await page.textContent('body')).includes(selected))}
  finally{await db.query('UPDATE platform_actor_state SET enabled=true,revision=revision+1 WHERE actor_id=$1',[selected])}
  await page.reload();await expect(userDetail().locator('dd').nth(2)).toHaveText('已关联');
 });
 await pass('users remain workspace scoped after switch and unconfigured identity source stays explicit',async()=>{
  await login(two,'双空间查询人员',b);await two.goto(p.origin+'/settings/users');await expect(two.getByRole('table',{name:'用户目录'}).locator('tbody tr')).toHaveCount(17);assert.ok(!(await two.textContent('body')).includes('空间甲成员'));await two.goto(userURL);await expect(two.getByRole('heading',{name:'指定用户不可见',exact:true})).toBeVisible();await two.goto(p.origin+'/settings/users?userId=person-b-2');await expect(two.getByRole('region',{name:'用户详情'})).toContainText('关联来源未配置');
 });
 await pass('user detail returned after token revocation during native profile read is discarded',async()=>{
  const direct=await browser.newContext({ignoreHTTPSErrors:true}),issued=await control({issueSubject:'admin-a',holdUser:true});let pending;
  try{pending=direct.request.get('https://localhost:18499/api/v1/workspaces/'+a+'/management/users?userId=person-a-2',{headers:{Authorization:'Bearer '+issued.testAccessToken}});await expect.poll(async()=>(await control({})).userWaiting,{timeout:3000}).toBe(1);await control({revokeSubject:'admin-a',holdUser:false});const response=await pending;assert.equal(response.status(),401);assert.equal(Object.hasOwn(await response.json(),'data'),false);await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await userDetail().count(),0);await login(page,'查询人员 A')}
  finally{await control({holdUser:false});if(pending)await pending.catch(()=>{});await direct.close()}
 });
 await pass('membership removed during account lookup cannot publish an orphan profile',async()=>{
  const direct=await browser.newContext({ignoreHTTPSErrors:true}),issued=await control({issueSubject:'admin-a',holdUser:true});let pending;
  try{pending=direct.request.get('https://localhost:18499/api/v1/workspaces/'+a+'/management/users?userId=person-a-2',{headers:{Authorization:'Bearer '+issued.testAccessToken}});await expect.poll(async()=>(await control({})).userWaiting,{timeout:3000}).toBe(1);await control({removeUser:true,holdUser:false});const response=await pending;assert.equal(response.status(),404);assert.equal(Object.hasOwn(await response.json(),'data'),false)}
  finally{await control({holdUser:false,removeUser:false});if(pending)await pending.catch(()=>{});await direct.close()}
 });
 const overview=()=>page.getByRole('region',{name:'工作空间概览',exact:true});
 await pass('workspace overview projects current authority and configuration without calling native directory',async()=>{
  const before=await control({});await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();await expect(overview().getByRole('heading',{name:'管理工作空间 A',exact:true})).toBeVisible();await expect(overview()).toContainText('接入配置');await expect(overview()).toContainText('尚未接入');await expect(overview()).toContainText('负责人、配额尚无可信资料');await page.reload();await expect(overview()).toBeVisible();const after=await control({});assert.equal(after.reads,before.reads);assert.equal(after.userReads,before.userReads);await expect(overview().getByText('连接正常',{exact:true})).toHaveCount(0);await page.screenshot({path:fileURLToPath(new URL('screenshots/workspace.png',run)),fullPage:true});
 });
 await pass('workspace member flow restores page filters row and workspace return through detail',async()=>{
  await overview().getByRole('link',{name:/^空间成员/}).click();await expect(rows(page)).toHaveCount(25);await page.getByRole('textbox',{name:'搜索成员',exact:true}).fill('空间甲成员');await page.getByRole('combobox',{name:'状态',exact:true}).selectOption('active');await page.getByRole('button',{name:'筛选',exact:true}).click();await page.getByRole('link',{name:'下一页',exact:true}).click();const list=page.url();assert.equal(new URL(list).searchParams.get('from'),'workspace');const id=await rows(page).last().getAttribute('id');await rows(page).last().getByRole('link',{name:'查看成员',exact:true}).click();await page.reload();await expect(page.getByRole('link',{name:'返回工作空间',exact:true})).toBeVisible();await page.getByRole('link',{name:'返回成员列表',exact:true}).click();assert.equal(new URL(page.url()).search,new URL(list).search);await expect(page.locator('tr:target')).toHaveAttribute('id',id);await page.getByRole('link',{name:'清除条件',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('from'),'workspace');await page.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(overview()).toBeVisible();
 });
 await pass('workspace users and role member links retain only the fixed workspace origin',async()=>{
  await overview().getByRole('link',{name:/^用户目录/}).click();await expect(userRows()).toHaveCount(25);await userRows().first().getByRole('link',{name:'查看用户',exact:true}).click();await page.reload();await expect(userDetail()).toBeVisible();await expect(page.getByRole('link',{name:'返回工作空间',exact:true})).toBeVisible();await userDetail().getByRole('link',{name:'查看角色权限',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('from'),'workspace');await page.getByRole('table',{name:'角色权限'}).getByRole('row').filter({hasText:'管理查询员'}).getByRole('link',{name:'查看相关成员',exact:true}).click();await expect(rows(page)).toHaveCount(25);assert.equal(new URL(page.url()).searchParams.get('from'),'workspace');await page.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(overview()).toBeVisible();
 });
 await pass('workspace audit cursor paging latest and refresh retain current-scope return',async()=>{
  await overview().getByRole('link',{name:/^审计记录/}).click();await expect(page.getByRole('table',{name:'空间审计记录'}).locator('tbody tr')).toHaveCount(25);await page.getByRole('link',{name:'更早记录',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('from'),'workspace');await page.reload();await page.getByRole('link',{name:'最新记录',exact:true}).click();assert.equal(new URL(page.url()).searchParams.has('cursor'),false);assert.equal(new URL(page.url()).searchParams.get('from'),'workspace');await page.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(overview()).toBeVisible();
 });
 await pass('native source outage is distinct from workspace authority and configured entry readiness',async()=>{
  try{await control({fail:true});const state=await control({});await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();assert.equal((await control({})).reads,state.reads);await overview().getByRole('link',{name:/^空间成员/}).click();await expect(page.getByRole('heading',{name:'成员目录暂时无法读取',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);await page.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(overview()).toBeVisible()}
  finally{await control({fail:false})}
 });
 await pass('workspace switch reauthenticates and fixed return never resurrects the previous space',async()=>{
  await login(two,'双空间查询人员',a);await two.goto(p.origin+'/settings/workspaces');await expect(two.getByRole('region',{name:'工作空间概览'})).toContainText('管理工作空间 A');await two.getByRole('link',{name:'切换与核验身份 →',exact:true}).click();await two.getByRole('link',{name:'管理工作空间 B',exact:true}).click();await two.getByRole('button',{name:'使用统一身份登录',exact:true}).click();await two.getByRole('button',{name:'双空间查询人员',exact:true}).click();await two.waitForURL(p.origin+'/**');await two.goto(p.origin+'/settings/workspaces');await expect(two.getByRole('region',{name:'工作空间概览'})).toContainText('管理工作空间 B');assert.ok(!(await two.textContent('body')).includes('管理工作空间 A'));
  await two.goto(p.origin+'/settings/users?userId=person-a-2&from=workspace');await expect(two.getByRole('heading',{name:'指定用户不可见',exact:true})).toBeVisible();await two.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(two.getByRole('region',{name:'工作空间概览'})).toContainText('管理工作空间 B');
  for(const path of ['/settings/workspaces?workspaceId='+a,'/settings/members?from=https%3A%2F%2Fexternal.invalid']){await two.goto(p.origin+path);await expect(two.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible();assert.equal(await two.getByRole('region',{name:'工作空间概览'}).count(),0)}
 });
 await pass('workspace revocation and expired credentials discard overview and cannot revive old sessions',async()=>{
  const actor='83000000-0000-4000-8000-000000000001';await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();await overview().getByRole('link',{name:/^空间成员/}).click();await expect(rows(page)).toHaveCount(25);
  try{await db.query('UPDATE platform_workspace_grant SET enabled=false,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,a]);await page.goBack();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await overview().count(),0);await page.goForward();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await rows(page).count(),0);await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();await page.screenshot({path:fileURLToPath(new URL('screenshots/workspace-revoked.png',run)),fullPage:true})}
  finally{await db.query('UPDATE platform_workspace_grant SET enabled=true,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,a])}
  await page.goto(p.origin+'/settings/workspaces');await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();await login(page,'查询人员 A');await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();await control({expireSubject:'admin-a'});await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();assert.equal(await overview().count(),0);await login(page,'查询人员 A');await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();
 });
 await pass('mobile workspace query and return controls stay reachable without page overflow',async()=>{
  await page.setViewportSize({width:390,height:844});await page.goto(p.origin+'/settings/workspaces');await expect(overview()).toBeVisible();assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:fileURLToPath(new URL('screenshots/workspace-mobile.png',run)),fullPage:true});await overview().getByRole('link',{name:/^用户目录/}).click();await expect(userRows()).toHaveCount(25);await page.getByRole('link',{name:'返回工作空间',exact:true}).click();await expect(overview()).toBeVisible();await page.setViewportSize({width:1600,height:950});
 });
 await pass('management profile has no business, model, Agent or write routes',async()=>{
  for(const path of ['/settings/members','/api/agent-tasks','/api/v1/business-acceptances','/core/acceptance-context'])assert.equal((await one.context.request.post(p.origin+path,{data:{workspaceId:a}})).status(),403);
  assert.equal((await one.context.request.get(p.origin+'/agent')).status(),403);assert.equal((await one.context.request.get('https://localhost:18499/api/v1/workspaces/'+a+'/management/members')).status(),400);
 });
 await pass('mobile management and read-only history remain intact after queries',async()=>{
  await page.setViewportSize({width:390,height:844});await page.goto(p.origin+'/settings/users');await expect(userRows()).toHaveCount(25);assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:fileURLToPath(new URL('screenshots/mobile.png',run)),fullPage:true});assert.equal(await businessHash(),before);for(const table of ['execution','agent_task','agent_action_request','delivery_provider_attempt','outbox_event'])assert.equal(Number((await db.query('SELECT count(*) AS n FROM '+table)).rows[0].n),0);
 });
 assert.deepEqual(errors,[]);assert.deepEqual(writes,[]);assert.deepEqual(external,[]);
}catch(error){process.exitCode=1;results.push({name:'failure near '+active,status:'FAIL',error:String(error).slice(0,1300)});console.log('FAIL isolated management browser; non-secret failure details in result file');}
finally{await control({permission:true,foreign:false,fail:false,holdList:false,userPermission:true,userForeign:false,userFail:false,userUnknown:false,holdUser:false,removeUser:false}).catch(()=>{});writeFileSync(new URL('browser-results.json',run),JSON.stringify({recordedAt:new Date().toISOString(),browser:channel,source:'SYNTHETIC_FIXTURE',realSources:'UNCONFIGURED',businessWrites:false,models:false,dispatch:false,errors,writes,external,verdict:process.exitCode?'FAIL':'PASS',results},null,2));await browser.close();await db.end()}
