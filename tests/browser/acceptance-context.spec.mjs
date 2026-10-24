import {test,expect} from '@playwright/test';import {ids,fixtureToken} from './fixture-server.mjs';
import {bindingId} from '../../apps/web/tests/fixtures/acceptance-bindings.mjs';
const control='http://127.0.0.1:4400/__control';
async function reset(request,scenario='acceptance-sources'){expect((await request.post(control+'/reset',{headers:{'x-fixture-token':fixtureToken},data:{scenario}})).ok()).toBeTruthy();}
async function calls(request){return(await(await request.get(control+'/state',{headers:{'x-fixture-token':fixtureToken}})).json()).requests;}
const target=`deliveryOperationId=${ids.deliveryOperation}&datasetVersionId=${ids.goldVersion}`;
test('standard selection, exact target, pagination, refresh and return are read only',async({page,request})=>{
 await reset(request);await page.goto('/core/acceptance-context?'+target);
 await expect(page.getByRole('heading',{name:'标准与验收责任',exact:true})).toBeVisible();await expect(page.getByText('业务责任关系尚未配置',{exact:true})).toBeVisible();
 await page.getByRole('link',{name:'查看标准 · 1.0.0',exact:true}).click();await expect(page).toHaveURL(new RegExp(`contractVersionId=${ids.version}.*${target}`));await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();
 await page.getByRole('link',{name:'下一页',exact:true}).click();await expect(page).toHaveURL(/offset=25$/);await expect(page.getByRole('link',{name:'查看标准 · 1.0.25',exact:true})).toBeVisible();await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();
 await page.reload();await expect(page.getByRole('link',{name:'查看标准 · 1.0.25',exact:true})).toBeVisible();await page.getByRole('link',{name:ids.deliveryOperation,exact:true}).click();await expect(page).toHaveURL('/core/deliveries?id='+ids.deliveryOperation);
 expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('persisted responsibility and applicability facts retain exact history through paging and refresh',async({page,request})=>{
 await reset(request,'acceptance-binding-history');await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version);
 await expect(page.getByText('标准适用关系已核验',{exact:true})).toBeVisible();await expect(page.getByRole('table',{name:'业务责任绑定'}).getByText('责任绑定有效',{exact:true})).toHaveCount(6);
 await page.getByRole('link',{name:'追溯标准确认人',exact:true}).click();await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText(bindingId(13));
 await page.getByRole('link',{name:'下一页绑定',exact:true}).click();await expect(page).toHaveURL(/bindingOffset=25$/);await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText(bindingId(13));await expect(page.getByText('标准适用关系已核验',{exact:true})).toBeVisible();
 await page.reload();await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText(bindingId(13));await page.getByRole('link',{name:'返回当前历史页'}).click();await expect(page).not.toHaveURL(/bindingId=/);await expect(page.getByRole('navigation',{name:'绑定历史分页'})).toContainText('偏移 25');
 await page.getByRole('link',{name:'上一页绑定'}).click();await page.getByRole('link',{name:'追溯标准适用绑定'}).click();await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText(bindingId(21));await expect(page.getByRole('main').getByRole('button',{name:/提交|确认通过|归档/})).toHaveCount(0);expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('expired representatives and effective revocation block dependent responsibilities while history survives',async({page,request})=>{
 await reset(request,'acceptance-binding-expired');await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version);const row=page.getByRole('table',{name:'业务责任绑定'}).getByRole('row').filter({has:page.getByRole('rowheader',{name:'接收方代表',exact:true})});await expect(row).toContainText('已失效或尚未生效');await page.getByRole('link',{name:'追溯接收方代表'}).click();await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText(bindingId(12));
 await reset(request,'acceptance-binding-revoked');await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version+'&bindingId='+bindingId(30));await expect(page.getByText('来源或确认人不匹配',{exact:true})).toBeVisible();await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText('REVOKED');await expect(page.getByRole('article',{name:'绑定事实详情'})).toContainText('合成夹具：代表资格撤销');expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('ambiguity and actor separation are blocked; changed criteria or confirmer never inherit old applicability',async({page,request})=>{
 for(const [scenario,message]of [['ambiguous','同一职责有多个有效绑定，须明确处置后再办理'],['conflict','提供方不能同时担任标准确认人或业务验收人'],['mismatch','标准来源、完整标准集或原确认责任已变化，须重新确认'],['replaced','标准来源、完整标准集或原确认责任已变化，须重新确认']]){await reset(request,'acceptance-binding-'+scenario);await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version);await expect(page.getByText(message,{exact:true}).first()).toBeVisible();await expect(page.getByRole('status')).toHaveCount(0);expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);}
 await page.getByRole('link',{name:'追溯标准适用绑定'}).click();const article=page.getByRole('article',{name:'绑定事实详情'});await expect(article).toContainText(bindingId(13));await expect(article).toContainText('关联依据与处置');await article.locator('summary').filter({hasText:bindingId(33)}).click();await expect(article).toContainText('合成夹具：更换确认责任');await expect(article).toContainText('SUPERSEDED');await expect(article).toContainText('主体代表关系');expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('foreign or missing exact binding never falls back to current; standard and list remain independently readable',async({page,request})=>{
 await reset(request,'acceptance-binding-current');await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version+'&bindingId='+ids.product);await expect(page.getByRole('status')).toContainText('UNKNOWN');await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();await expect(page.getByText('标准适用关系已核验',{exact:true})).toHaveCount(0);await expect(page.getByRole('article',{name:'绑定事实详情'})).toHaveCount(0);expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('invalid target does not read APIs; foreign selected standard remains UNKNOWN beside known list',async({page,request})=>{
 await reset(request);await page.goto('/core/acceptance-context?deliveryOperationId=DEMO-1');await expect(page.getByRole('heading',{name:'验收准备引用无效'})).toBeVisible();expect(await calls(request)).toHaveLength(0);
 await reset(request,'acceptance-source-foreign');await page.goto('/core/acceptance-context?contractVersionId='+ids.version);await expect(page.getByRole('status')).toContainText('UNKNOWN');await expect(page.getByRole('link',{name:'查看标准 · 1.0.0',exact:true})).toBeVisible();expect((await calls(request)).filter(c=>c.path.startsWith('/api/v1/contract-versions/')).map(c=>c.path)).toEqual(['/api/v1/contract-versions/'+ids.version]);
});
test('responsibility failure and source failure retain independent known regions without synthetic fallback',async({page,request})=>{
 await reset(request,'acceptance-context-denied');await page.goto('/core/acceptance-context?'+target+'&contractVersionId='+ids.version);await expect(page.getByRole('status')).toContainText('UNKNOWN');await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();
 await reset(request,'acceptance-list-denied');await page.reload();await expect(page.getByRole('status')).toContainText('UNKNOWN');await expect(page.getByText('业务责任关系尚未配置',{exact:true})).toBeVisible();await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('unsupported standard and unexpected private fields never become acceptance authority',async({page,request})=>{
 await reset(request,'acceptance-source-invalid');await page.goto('/core/acceptance-context?contractVersionId='+ids.version);await expect(page.getByText('缺少结构化验收标准',{exact:true})).toBeVisible();await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toHaveCount(0);
 await reset(request,'acceptance-source-extra');await page.reload();await expect(page.getByRole('cell',{name:'C1 · 核对明确成果范围',exact:true})).toBeVisible();expect(await page.content()).not.toContain('private-acceptance-source-sentinel');expect(await page.content()).not.toContain('private-acceptance-location');await expect(page.getByRole('main').getByRole('button',{name:/提交|确认通过|归档/})).toHaveCount(0);expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
