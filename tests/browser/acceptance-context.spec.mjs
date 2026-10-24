import {test,expect} from '@playwright/test';import {ids,fixtureToken} from './fixture-server.mjs';
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
