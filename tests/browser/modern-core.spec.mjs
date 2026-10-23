import {test,expect} from '@playwright/test';
import {ids,fixtureToken} from './fixture-server.mjs';
const control='http://127.0.0.1:4400/__control';
async function reset(request,scenario='modern-trace'){expect((await request.post(control+'/reset',{headers:{'x-fixture-token':fixtureToken},data:{scenario}})).ok()).toBeTruthy();}
async function calls(request){return (await(await request.get(control+'/state',{headers:{'x-fixture-token':fixtureToken}})).json()).requests;}
test('prototype root stays synthetic; Core workbench uses server facts in the same modern shell',async({page,request})=>{
 await reset(request,'ready');await page.goto('/');
 await expect(page.getByText('本地交互演示',{exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:'重置演示',exact:true})).toBeVisible();
 expect(await calls(request)).toHaveLength(0);
 await page.getByRole('button',{name:'Core 待办',exact:true}).click();
 await expect(page).toHaveURL('/core/workbench');
 await expect(page.getByTestId('workbench-attention-total')).toBeVisible();
 await expect(page.getByRole('button',{name:'重置演示',exact:true})).toHaveCount(0);
 expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('exact release trace shows safe historical facts, survives refresh and preserves return filter',async({page,request})=>{
 await reset(request);const url=`/core/release-trace?releaseId=${ids.release}&returnStatus=PUBLISHED&returnOffset=25`;
 await page.goto(url);await expect(page.getByRole('heading',{name:'SYNTHETIC_TRACE · PUBLISHED',exact:true})).toBeVisible();
 await expect(page.getByText('a'.repeat(64),{exact:true})).toBeVisible();
 await expect(page.getByText('INTEGRITY_OK',{exact:true})).toBeVisible();
 await page.locator('summary').filter({hasText:ids.decision}).click();await expect(page.getByText(ids.decision,{exact:true})).toBeVisible();
 await page.locator('summary').filter({hasText:ids.deliveryOperation}).click();await expect(page.getByText('0.000000000000000001',{exact:true})).toBeVisible();
 await page.locator('summary').filter({hasText:ids.goldVersion}).click();
 await expect(page.getByRole('link',{name:'查看明确版本与质量',exact:true})).toHaveAttribute('href',`/core/assets?datasetId=${ids.goldDataset}&versionId=${ids.goldVersion}`);
 await test.info().attach('modern-release-trace',{body:await page.screenshot({fullPage:true}),contentType:'image/png'});
 await page.reload();await expect(page.getByRole('heading',{name:'SYNTHETIC_TRACE · PUBLISHED',exact:true})).toBeVisible();
 await page.getByRole('link',{name:'返回发布历史',exact:true}).click();await expect(page).toHaveURL(new RegExp(`offset=25&status=PUBLISHED&releaseId=${ids.release}$`));
 expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
test('cross-workspace or missing release never falls back to a known release',async({page,request})=>{
 await reset(request,'modern-trace-foreign');await page.goto(`/core/release-trace?releaseId=${ids.release}`);
 await expect(page.getByRole('status')).toContainText('UNKNOWN');await expect(page.getByText('SYNTHETIC_TRACE',{exact:true})).toHaveCount(0);
 await reset(request);await page.goto(`/core/release-trace?releaseId=${ids.createdRelease}`);
 await expect(page.getByRole('status')).toContainText('UNKNOWN');
 const reads=await calls(request);expect(reads.filter(c=>c.path.startsWith('/api/v1/traceability/')).map(c=>c.path)).toEqual([`/api/v1/traceability/product-releases/${ids.createdRelease}`]);
 expect(reads.some(c=>c.method!=='GET')).toBe(false);
});

test('frontend field projection does not render unexpected upstream storage or source fields',async({page,request})=>{
 await reset(request,'modern-trace-extra');await page.goto(`/core/release-trace?releaseId=${ids.release}`);
 await expect(page.getByRole('heading',{name:'SYNTHETIC_TRACE · PUBLISHED',exact:true})).toBeVisible();
 for(const detail of await page.locator('details').all())await detail.locator('summary').click();
 expect(await page.content()).not.toContain('private-trace-storage-sentinel');
 expect(await page.content()).not.toContain('private-trace-source-key');
 expect((await calls(request)).some(c=>c.method!=='GET')).toBe(false);
});
