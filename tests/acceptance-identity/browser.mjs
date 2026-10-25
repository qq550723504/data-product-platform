// Actual Edge, isolated contexts, synthetic credentials never logged/exported.
import {readFileSync,mkdirSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {resolve,dirname} from 'node:path';
import assert from 'node:assert/strict';
import {createHash,randomUUID} from 'node:crypto';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../..');
const require=createRequire(resolve(root,'tests/browser/package.json'));
const {chromium}=require('playwright');
const {expect}=require('@playwright/test');
const run=resolve(root,'.data/acceptance-identity-1a');
const profile=JSON.parse(readFileSync(resolve(run,'profile.json'),'utf8'));
assert.equal(profile.database,'acceptance_1a_20261024');assert.equal(profile.origin,'https://localhost:4290');
const {Pool}=createRequire(resolve(root,'apps/web/package.json'))('pg');
const db=new Pool({host:'127.0.0.1',port:25495,user:'postgres',password:profile.pgPassword,database:profile.database,ssl:{ca:readFileSync(resolve(run,'ca.crt'),'utf8'),rejectUnauthorized:true}});
const origin=profile.origin, a='11111111-1111-4111-8111-111111111111', b='12121212-1212-4212-8212-121212121212';
const results=[];mkdirSync(resolve(run,'screenshots'),{recursive:true});
const channel=process.env.ACCEPTANCE_BROWSER_CHANNEL??'msedge';
if(!['msedge','chromium'].includes(channel))throw new Error('Explicit Edge or CI Chromium required');
const browser=await chromium.launch({...(channel==='msedge'?{channel}:{}),headless:true});
const contexts=[];
const people=new Map();
async function protectedFacts(){return (await db.query(`SELECT md5(string_agg(v,'|' ORDER BY v)) AS hash FROM (
 SELECT 'delivery:'||row_to_json(t)::text AS v FROM delivery_operation t UNION ALL
 SELECT 'standard:'||row_to_json(t)::text FROM contract_version t UNION ALL
 SELECT 'representation:'||row_to_json(t)::text FROM acceptance_party_representation t UNION ALL
 SELECT 'role:'||row_to_json(t)::text FROM acceptance_responsibility_binding t UNION ALL
 SELECT 'applicability:'||row_to_json(t)::text FROM acceptance_standard_applicability t UNION ALL
 SELECT 'disposition:'||row_to_json(t)::text FROM acceptance_binding_disposition t UNION ALL
 SELECT 'evidence:'||row_to_json(t)::text FROM evidence t UNION ALL
 SELECT 'evidence-relation:'||row_to_json(t)::text FROM evidence_relation t) f`)).rows[0].hash;}
async function noExecution(){for(const table of ['execution','agent_task','agent_action_request','delivery_provider_attempt','outbox_event']){
 const exists=(await db.query('SELECT to_regclass($1) AS name',[table])).rows[0].name;if(!exists)continue;
 assert.equal(Number((await db.query('SELECT count(*) AS n FROM '+table)).rows[0].n),0,table+' must remain empty');
}}
async function person(name,workspace=a){
 const context=await browser.newContext({ignoreHTTPSErrors:true,serviceWorkers:'block',viewport:{width:1440,height:1000}});contexts.push(context);
 await context.route('**/*',route=>{const u=new URL(route.request().url());return [origin,new URL(profile.issuer).origin].includes(u.origin)?route.continue():route.abort();});
 const page=await context.newPage();
 await page.goto(origin+'/auth/login?workspaceId='+workspace);
 await page.getByRole('button',{name:'使用统一身份登录'}).click();
 await page.waitForURL('https://localhost:19120/**');
 await page.locator('#username').fill(profile.people[name].username);
 await page.locator('#password').fill(profile.people[name].password);
 await page.locator('#kc-login').click();
 await page.waitForURL(origin+'/**',{timeout:30000});
 return {context,page,name};
}
let activeCase='login/setup';
async function pass(name,fn){activeCase=name;await fn();results.push({name,status:'PASS'});console.log('PASS '+name);}
try{
 const before=await protectedFacts();await noExecution();
 for(const name of ['provider','receiver','confirmer']){
  const p=await person(name);people.set(name,p);
  await pass(name+' separate OIDC session and canonical mapping',async()=>{
   assert.equal(await p.page.getByTestId('current-principal').textContent(),profile.people[name].actor);
   assert.equal(await p.page.getByTestId('current-workspace').textContent(),a);
   const cookies=await p.context.cookies(origin);const session=cookies.find(c=>c.name==='__Host-dpp-session');
   assert.ok(session?.secure&&session.httpOnly&&session.sameSite==='Lax');
  });
  const target=p.page.locator('a[href*="deliveryOperationId=74000000-0000-4000-8000-000000001002"]');
  await pass(name+' current target responsibility and writes disabled',async()=>{
   assert.equal(await target.count(),1);await target.click();
   await p.page.getByRole('heading',{name:'本人责任与办理准入'}).waitFor();
   const panel=p.page.getByRole('region',{name:'事务内身份与责任预检'});
   assert.ok((await panel.textContent()).includes('本人责任已匹配'));
   assert.equal(await panel.getByText('业务写入口保持关闭',{exact:true}).count(),4);
   assert.ok((await panel.textContent()).includes('合成夹具'));
   await p.page.reload();await p.page.getByRole('heading',{name:'本人责任与办理准入'}).waitFor();
   await p.page.getByRole('link',{name:'查看标准 · 1.0.0',exact:true}).click();
   await p.page.getByText('标准适用关系已核验',{exact:true}).waitFor();
   await p.page.getByRole('link',{name:'追溯标准确认人',exact:true}).click();
   await p.page.getByRole('heading',{name:'绑定事实详情',exact:true}).waitFor();
   await p.page.reload();await p.page.getByRole('heading',{name:'绑定事实详情',exact:true}).waitFor();
  });
  await p.page.screenshot({path:resolve(run,'screenshots',name+'-admission.png'),fullPage:true});
  await pass(name+' business API / legacy bypass denied',async()=>{
   for(const path of ['/api/v1/business-acceptances','/api/agent/tasks','/demo','/_next/data/test/identity.json']){
    const r=await p.context.request.post(origin+path,{data:{workspaceId:a}});assert.equal(r.status(),403);
   }
  });
 }
 const basisURL=(workspace=a,extra={})=>origin+'/core/acceptance-basis?'+new URLSearchParams({deliveryOperationId:workspace===a?'74000000-0000-4000-8000-000000001002':'74000000-0000-4000-8000-000000002002',datasetVersionId:workspace===a?'74000000-0000-4000-8000-000000001001':'74000000-0000-4000-8000-000000002001',contractVersionId:workspace===a?'74000000-0000-4000-8000-000000001004':'74000000-0000-4000-8000-000000002004',...extra});
 for(const name of ['provider','receiver','confirmer']){
  const p=people.get(name);
  await pass(name+' business basis separates identity, recorded responsibility, standard and unconfigured authority',async()=>{
   await p.page.getByRole('link',{name:'核对业务依据与收件材料',exact:true}).click();
   await p.page.getByRole('heading',{name:'业务依据核对',exact:true}).waitFor();
   assert.equal(await p.page.getByRole('region',{name:'人员与责任依据'}).count(),1);
   assert.equal(await p.page.getByRole('region',{name:'交付与标准依据'}).count(),1);
   await expect(p.page.getByText('查看代表资格依据记录',{exact:true})).toHaveCount(6);
   await expect(p.page.getByText('查看标准适用依据记录',{exact:true})).toHaveCount(1);
   assert.ok((await p.page.textContent('body')).includes('平台标准适用绑定匹配'));
   assert.ok((await p.page.textContent('body')).includes('当前交付为 已阻断'));
   assert.equal(await p.page.getByRole('button').count(),0);
  });
  await pass(name+' material pagination/detail/refresh and context return retain exact identities',async()=>{
   await p.page.getByRole('link',{name:'下一页材料',exact:true}).click();
   await p.page.waitForURL(u=>u.searchParams.get('materialOffset')==='25');
   const list=p.page.getByRole('region',{name:'收件材料引用'});
   await expect(list.locator('article')).toHaveCount(3);
   await list.locator('article').filter({hasText:'74000000-0000-4000-8000-000000001100'}).getByRole('link',{name:'查看核对详情'}).click();
   await expect(p.page.getByRole('heading',{name:'材料核对详情',exact:true})).toBeInViewport();await p.page.reload();
   const detail=p.page.getByRole('region',{name:'材料核对详情'});
   assert.ok((await detail.textContent()).includes('平台记录完整'));
   assert.ok((await detail.textContent()).includes('尚不能用于业务签收'));
   assert.ok((await detail.textContent()).includes('附件原始内容及其摘要尚无可信证明'));
   await p.page.getByRole('link',{name:'核对标准版本与责任历史',exact:true}).click();
   const back=p.page.getByRole('link',{name:'核对业务依据与收件材料',exact:true});
   await expect(back).toHaveAttribute('href',/materialOffset=25/);
   await back.click();await p.page.waitForURL(u=>u.pathname==='/core/acceptance-basis'&&u.searchParams.get('materialOffset')==='25');
   assert.equal(new URL(p.page.url()).searchParams.get('materialOffset'),'25');
   assert.equal(new URL(p.page.url()).searchParams.get('materialId'),'74000000-0000-4000-8000-000000001100');
   await p.page.getByRole('region',{name:'材料核对详情'}).waitFor();
   await expect(p.page.getByRole('heading',{name:'材料核对详情',exact:true})).toBeInViewport();
   const html=await p.page.content();for(const marker of ['PRIVATE_BASIS_METADATA','PRIVATE_TITLE','PRIVATE_URI'])assert.equal(html.includes(marker),false);
  });
 }
 const evidencePage=people.get('receiver').page;
 await pass('broken, oversized and governance records stay blocked',async()=>{
  for(const [materialId,text] of [['74000000-0000-4000-8000-000000001102','平台记录损坏'],['74000000-0000-4000-8000-000000001103','记录未核验'],['74000000-0000-4000-8000-000000001101','治理结果或其他记录不能作为业务收到声明']]){
   await evidencePage.goto(basisURL(a,{materialId,materialOffset:'25'}));const detail=evidencePage.getByRole('region',{name:'材料核对详情'});await detail.waitFor();assert.ok((await detail.textContent()).includes(text));assert.ok((await detail.textContent()).includes('尚不能用于业务签收'));
  }
 });
 await pass('foreign, missing and one-sided material references are UNKNOWN without replacing known overview',async()=>{
  for(const materialId of ['74000000-0000-4000-8000-000000002100','74000000-0000-4000-8000-000000001104','74000000-0000-4000-8000-000000001105','74000000-0000-4000-8000-000000009999']){
   await evidencePage.goto(basisURL(a,{materialId,materialOffset:'25'}));assert.ok((await evidencePage.getByRole('region',{name:'材料核对详情'}).textContent()).includes('无法确认'));
   assert.equal(await evidencePage.getByRole('region',{name:'收件材料引用'}).locator('article').count(),3);
  }
 });
 await evidencePage.goto(basisURL());await evidencePage.screenshot({path:resolve(run,'screenshots','business-basis-desktop.png'),fullPage:true});
 const foreign=await person('provider',b);
 await pass('provider has no grant in workspace B even with responsibility facts',async()=>{
  assert.equal(await foreign.page.getByTestId('current-principal').count(),0);
  assert.ok((await foreign.page.textContent('body')).includes('登录未完成'));
 });
 const receiver=await person('receiver',b);
 await pass('receiver independently authorized in workspace B',async()=>{
  assert.equal(await receiver.page.getByTestId('current-workspace').textContent(),b);
  await receiver.page.locator('a[href*="deliveryOperationId=74000000-0000-4000-8000-000000002002"]').click();
  await receiver.page.getByRole('heading',{name:'本人责任与办理准入'}).waitFor();
 });
 await pass('foreign target URL never returns workspace A data',async()=>{
  await receiver.page.goto(origin+'/core/acceptance-context?deliveryOperationId=74000000-0000-4000-8000-000000001002&datasetVersionId=74000000-0000-4000-8000-000000001001');
  assert.equal(await receiver.page.getByRole('heading',{name:'本人责任与办理准入'}).count(),0);
 });
 await pass('business basis and materials retain workspace B boundary',async()=>{
  await receiver.page.goto(basisURL(b,{materialId:'74000000-0000-4000-8000-000000002100'}));
  assert.ok((await receiver.page.getByRole('region',{name:'材料核对详情'}).textContent()).includes('平台记录完整'));
  await receiver.page.goto(basisURL(b,{materialId:'74000000-0000-4000-8000-000000001100'}));
  assert.ok((await receiver.page.getByRole('region',{name:'材料核对详情'}).textContent()).includes('无法确认'));
  await receiver.page.goto(basisURL(a));assert.equal(await receiver.page.getByRole('region',{name:'人员与责任依据'}).count(),0);
 });
 const narrow=await person('confirmer');await narrow.page.setViewportSize({width:390,height:844});
 await narrow.page.screenshot({path:resolve(run,'screenshots','identity-narrow.png'),fullPage:true});
 await pass('narrow identity page has no horizontal overflow',async()=>assert.ok(await narrow.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)));
 await pass('narrow business basis remains readable without page overflow',async()=>{
  await narrow.page.goto(basisURL(a,{materialId:'74000000-0000-4000-8000-000000001100',materialOffset:'25'}));
  await narrow.page.getByRole('heading',{name:'材料核对详情',exact:true}).waitFor();assert.ok(await narrow.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await narrow.page.screenshot({path:resolve(run,'screenshots','business-basis-narrow.png'),fullPage:true});
 });
 for(const name of ['reviewer','archivist','disposer']){
  const p=await person(name);await pass(name+' has own read identity without future write capabilities',async()=>{
   assert.equal(await p.page.getByTestId('current-principal').textContent(),profile.people[name].actor);
   assert.equal(await p.page.getByText('查看验收责任与标准',{exact:true}).count(),1);
   assert.equal(await p.page.getByText('提供方草稿资格',{exact:true}).count(),0);
  });
 }
 await pass('all browser reads preserve immutable business facts and dispatch stays empty',async()=>{assert.equal(await protectedFacts(),before);await noExecution()});
 await pass('actor revocation rejects open session; reenabling never revives its revision',async()=>{
  const actor=profile.people.provider.actor,p=people.get('provider');
  try{
   await db.query('UPDATE platform_actor_state SET enabled=false,revision=revision+1 WHERE actor_id=$1',[actor]);await p.page.goto(origin+'/identity');
   assert.equal(await p.page.getByTestId('current-principal').count(),0);
  }finally{await db.query('UPDATE platform_actor_state SET enabled=true,revision=revision+1 WHERE actor_id=$1 AND NOT enabled',[actor])}
  await p.page.reload();assert.equal(await p.page.getByTestId('current-principal').count(),0);
  await p.page.goto(basisURL());assert.equal(await p.page.getByRole('region',{name:'人员与责任依据'}).count(),0);
  const fresh=await person('provider');assert.equal(await fresh.page.getByTestId('current-principal').textContent(),actor);
 });
 await pass('revoking only workspace B rejects B session while A remains available',async()=>{
  const actor=profile.people.receiver.actor;
  try{
   await db.query('UPDATE platform_workspace_grant SET enabled=false,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,b]);
   await receiver.page.goto(origin+'/identity');assert.equal(await receiver.page.getByTestId('current-principal').count(),0);
   await people.get('receiver').page.goto(origin+'/identity');assert.equal(await people.get('receiver').page.getByTestId('current-workspace').textContent(),a);
  }finally{await db.query('UPDATE platform_workspace_grant SET enabled=true,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2 AND NOT enabled',[actor,b])}
  await receiver.page.reload();assert.equal(await receiver.page.getByTestId('current-principal').count(),0);
 });
 await pass('database-expired browser session fails closed without static fallback',async()=>{
  const p=people.get('confirmer'),key=(await p.context.cookies(origin)).find(v=>v.name==='__Host-dpp-session').value;
  await db.query('UPDATE platform_browser_session SET expires_at=clock_timestamp()-interval \'1 second\' WHERE key_hash=$1',[createHash('sha256').update(key).digest()]);
  await p.page.goto(origin+'/identity');assert.equal(await p.page.getByTestId('current-principal').count(),0);
 });
 await pass('stored business history rejects update and delete',async()=>{
  for(const sql of ["UPDATE acceptance_responsibility_binding SET role='PROVIDER' WHERE id='74000000-0000-4000-8000-000000001041'","DELETE FROM acceptance_responsibility_binding WHERE id='74000000-0000-4000-8000-000000001041'"]){
   await assert.rejects(db.query(sql),e=>e.code==='23514');
  }assert.equal(await protectedFacts(),before);
 });
 await noExecution();
}catch(e){
 results.push({name:activeCase,status:'FAIL',kind:e?.name??'Error'});process.exitCode=1;
 let safe=String(e?.stack??e);for(const [key,value]of Object.entries(profile))if(typeof value==='string'&&/password|secret|encryption/i.test(key))safe=safe.replaceAll(value,'[REDACTED]');for(const p of Object.values(profile.people))safe=safe.replaceAll(p.password,'[REDACTED]');
 safe=safe.replace(/https?:\/\/\S+/g,'[URL]').replace(/(?:Bearer|Basic) \S+/g,'[AUTH]').replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g,'[TOKEN]');
 console.error('FAIL '+activeCase+'\n'+safe.slice(0,3000));
 for(const p of people.values())if(new URL(p.page.url()).pathname==='/core/acceptance-basis')await p.page.screenshot({path:resolve(run,'screenshots','basis-failure.png'),fullPage:true}).catch(()=>{});
}
finally{
 writeFileSync(resolve(run,'browser-results.json'),JSON.stringify({browser:channel==='msedge'?'Edge':'CI Chromium',results},null,2));
 for(const context of contexts)await context.close();await browser.close();await db.end();
}
