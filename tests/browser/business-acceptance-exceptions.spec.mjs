import {test,expect} from '@playwright/test';
import {fixtureToken} from './fixture-server.mjs';
const control='http://127.0.0.1:4400/__control';
const actor=(page,value)=>page.getByRole('combobox',{name:"办理身份（预览）",exact:true}).selectOption(value);
const note=(page,value)=>page.getByRole('textbox',{name:'办理说明',exact:true}).fill(value);
const click=(page,name)=>page.getByRole('button',{name,exact:true}).click();
const submissionRows=page=>page.getByRole('row').filter({hasText:/验收提交 /});
async function receipt(page,evidence){
 await click(page,'核对签收记录');await actor(page,'receiver');
 await page.getByRole('textbox',{name:'签收依据',exact:true}).fill(evidence);
 await page.getByRole('textbox',{name:'签收说明',exact:true}).fill('演示签收说明 '+evidence);
 await click(page,'保存签收记录');await click(page,'进入业务验收');
}
async function submit(page){await actor(page,'provider');await click(page,'确认冻结并提交验收');}
async function passCriteria(page){await actor(page,'reviewer');for(const name of ['字段与约定一致','统计期间符合需求','成果满足业务使用要求'])await page.getByRole('combobox',{name:'核验 '+name,exact:true}).selectOption('PASS');}
async function approve(page){await passCriteria(page);await note(page,'按冻结材料逐项验收通过');await click(page,'确认验收通过');}
test.beforeEach(async({page,request})=>{
 await request.post(control+'/reset',{headers:{'x-fixture-token':fixtureToken},data:{scenario:'ready'}});
 await page.goto('/#business-acceptances');await click(page,'查看验收');
});
test.afterEach(async({request})=>{const s=await(await request.get(control+'/state',{headers:{'x-fixture-token':fixtureToken}})).json();expect(s.requests).toHaveLength(0);});

test('withdrawal needs submitter and reason; resubmission and cancellation preserve exact submissions',async({page})=>{
 await receipt(page,'WITHDRAW-R1');await submit(page);
 await actor(page,'reviewer');await note(page,'不应允许验收人撤回');await expect(page.getByRole('button',{name:'撤回提交',exact:true})).toBeDisabled();
 await actor(page,'provider');await expect(page.getByRole('button',{name:'撤回提交',exact:true})).toBeDisabled();
 await note(page,'首次撤回补充材料');await click(page,'撤回提交');
 await expect(submissionRows(page).filter({hasText:'第1次'})).toContainText('已撤回');
 await submit(page);await expect(submissionRows(page)).toHaveCount(2);await expect(submissionRows(page).filter({hasText:'第2次'})).toContainText('待验收');
 await page.reload();await expect(submissionRows(page).filter({hasText:'第1次'})).toContainText('已撤回');
 await expect(page.getByRole('button',{name:'取消验收',exact:true})).toHaveCount(0);
 await note(page,'第二次撤回后取消');await click(page,'撤回提交');await note(page,'业务需求终止');await click(page,'取消验收');
 await page.reload();await expect(page.getByText(/修订.*已取消/)).toBeVisible();await expect(submissionRows(page)).toHaveCount(2);
 await expect(page.getByRole('button',{name:'确认冻结并提交验收',exact:true})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'冻结验收档案',exact:true})).toHaveCount(0);
 await click(page,'核对签收记录');await actor(page,'receiver');await expect(page.getByRole('button',{name:'保存签收记录',exact:true})).toBeDisabled();
});

test('draft cancellation requires reason and cannot invent acceptance or archive facts',async({page})=>{
 await expect(page.getByRole('button',{name:'取消验收',exact:true})).toBeDisabled();await note(page,'草稿不再办理');await click(page,'取消验收');
 await expect(page.getByText(/修订.*已取消/)).toBeVisible();await page.reload();
 await expect(submissionRows(page)).toHaveCount(0);await expect(page.getByRole('button',{name:'编辑草稿与标准',exact:true})).toHaveCount(0);
 await click(page,'验收归档');await expect(page.getByText(/尚无验收档案/)).toBeVisible();
});

test('receipt replacement blocks approval and can return on context change without a false criterion failure',async({page})=>{
 await receipt(page,'FROZEN-RECEIPT-A');await submit(page);await receipt(page,'REPLACEMENT-RECEIPT-B');
 await passCriteria(page);await note(page,'签收依据已变化，请重新提交');
 await expect(page.getByText('暂不能通过：签收依据已变化，请退回后重新提交',{exact:true})).toBeVisible();
 await expect(page.getByRole('button',{name:'确认验收通过',exact:true})).toBeDisabled();
 await expect(page.getByRole('button',{name:'退回修改',exact:true})).toBeEnabled();await click(page,'退回修改');
 const original=submissionRows(page).filter({hasText:'第1次'});await original.locator('summary').click();await expect(original).toContainText('FROZEN-RECEIPT-A');await expect(original).not.toContainText('REPLACEMENT-RECEIPT-B');await expect(original).toContainText("C1: 通过");
 await submit(page);const second=submissionRows(page).filter({hasText:'第2次'});await second.locator('summary').click();await expect(second).toContainText('REPLACEMENT-RECEIPT-B');
 await approve(page);await expect(page.getByText(/修订.*验收通过.*未归档/)).toBeVisible();
});

test('invalidating an archived decision preserves original facts and exact receipt/submission trace after refresh',async({page})=>{
 await receipt(page,'OLD-UNREFERENCED');await receipt(page,'ARCHIVED-RECEIPT');await submit(page);await approve(page);
 await actor(page,'archivist');await click(page,'冻结验收档案');const heading=page.getByRole('heading',{name:/验收档案 /});const archiveTitle=await heading.innerText();
 await click(page,'返回对应验收与办理历史');await actor(page,'reviewer');await expect(page.getByRole('button',{name:'追加决定作废处置',exact:true})).toBeDisabled();
 await note(page,'原审核依据存在错误，追加作废，不覆盖原决定');await click(page,'追加决定作废处置');
 await expect(page.getByText(/修订.*原通过决定已作废/)).toBeVisible();
 await click(page,'查看已归档记录');await expect(heading).toHaveText(archiveTitle);await expect(page.getByText('原决定已作废',{exact:true})).toBeVisible();
 await expect(page.getByRole('region',{name:'决定后续处置'})).toContainText('原审核依据存在错误');await expect(page.getByRole('region',{name:'决定后续处置'})).toContainText('DEMO-REVIEWER');
 await expect(page.getByText(/原决定：.*通过.*按冻结材料逐项验收通过/)).toBeVisible();
 await page.reload();await expect(heading).toHaveText(archiveTitle);await expect(page.getByText('原决定已作废',{exact:true})).toBeVisible();
 await click(page,'追溯归档引用的签收');const referenced=page.getByRole('row').filter({hasText:'归档引用'});await expect(referenced).toContainText('ARCHIVED-RECEIPT');await expect(referenced).not.toContainText('OLD-UNREFERENCED');
 await click(page,'进入业务验收');await expect(submissionRows(page).locator('details')).toHaveAttribute('open','');await expect(submissionRows(page)).toContainText('ARCHIVED-RECEIPT');
 await expect(submissionRows(page)).toContainText('已作废');await expect(page.getByRole('button',{name:'追加决定作废处置',exact:true})).toBeDisabled();
 await actor(page,'archivist');await click(page,'查看已归档记录');await expect(page.getByRole('button',{name:'打开归档',exact:true})).toHaveCount(1);
 await click(page,'返回对应验收与办理历史');await actor(page,'provider');await click(page,'新建验收草稿');
 await page.getByRole('textbox',{name:'验收名称',exact:true}).fill('追溯后新建独立验收');await click(page,'创建草稿');
 await expect(page.getByText('归档引用的明确提交未找到，没有替换为最新提交。',{exact:true})).toHaveCount(0);
 await expect(submissionRows(page)).toHaveCount(0);
});

test('switching receipt cases clears unsaved evidence and outcome before recording a different case',async({page})=>{
 await click(page,'新建验收草稿');await page.getByRole('textbox',{name:'验收名称',exact:true}).fill('第二张独立验收单');await click(page,'创建草稿');
 await click(page,'核对签收记录');await actor(page,'receiver');
 const selection=page.getByRole('combobox',{name:'对应验收单',exact:true}),secondId=await selection.inputValue();
 const evidence=page.getByRole('textbox',{name:'签收依据',exact:true}),receiptNote=page.getByRole('textbox',{name:'签收说明',exact:true}),outcome=page.getByRole('combobox',{name:'收到情况',exact:true});
 await outcome.selectOption('DISPUTED');await evidence.fill('UNSAVED-CASE-B');await receiptNote.fill('第二单尚未保存');
 await selection.selectOption('DEMO-ACC-1');await expect(evidence).toHaveValue('');await expect(receiptNote).toHaveValue('');await expect(outcome).toHaveValue('RECEIVED');
 await evidence.fill('SAVED-CASE-A');await receiptNote.fill('仅登记第一单');await click(page,'保存签收记录');
 await selection.selectOption(secondId);await expect(evidence).toHaveValue('');await expect(page.getByRole('row').filter({hasText:'SAVED-CASE-A'})).toHaveCount(0);
 await evidence.fill('SAVED-CASE-B');await receiptNote.fill('仅登记第二单');await click(page,'保存签收记录');
 await expect(page.getByRole('row').filter({hasText:'SAVED-CASE-B'})).toHaveCount(1);await expect(page.getByRole('row').filter({hasText:'UNSAVED-CASE-B'})).toHaveCount(0);
 await selection.selectOption('DEMO-ACC-1');await expect(page.getByRole('row').filter({hasText:'SAVED-CASE-A'})).toHaveCount(1);await expect(page.getByRole('row').filter({hasText:'SAVED-CASE-B'})).toHaveCount(0);
});

test('workspace switches isolate identical demo IDs, frozen archives, dispositions and unsaved review input',async({page})=>{
 const switchTo=async(name,route)=>{
  await page.getByRole('button',{name:/^切换工作空间，当前：/}).click();await page.getByRole('button',{name:new RegExp('^'+name)}).click();
  await expect(page.locator('.workspace-scope-bar strong')).toHaveText(name);await click(page,'数据开发中心');await click(page,route);
 };
 await receipt(page,'ENTERPRISE-RECEIPT');await submit(page);await approve(page);await actor(page,'archivist');await click(page,'冻结验收档案');
 const archiveTitle=await page.getByRole('heading',{name:/验收档案 /}).innerText();
 await switchTo('园区能耗分析','验收归档');await expect(page.getByText(/尚无验收档案/)).toBeVisible();await expect(page.getByText('ENTERPRISE-RECEIPT',{exact:false})).toHaveCount(0);
 await click(page,'业务验收');await click(page,'查看验收');await expect(page.getByText(/验收单 1 · 修订 0 · 草稿/)).toBeVisible();await expect(submissionRows(page)).toHaveCount(0);
 await click(page,'核对签收记录');await actor(page,'receiver');await expect(page.getByRole('textbox',{name:'签收依据',exact:true})).toHaveValue('');await expect(page.getByRole('textbox',{name:'签收说明',exact:true})).toHaveValue('');
 await page.getByRole('textbox',{name:'签收依据',exact:true}).fill('PARK-RECEIPT');await page.getByRole('textbox',{name:'签收说明',exact:true}).fill('园区独立签收');await click(page,'保存签收记录');await click(page,'进入业务验收');await submit(page);
 await actor(page,'reviewer');await page.getByRole('combobox',{name:'核验 字段与约定一致',exact:true}).selectOption('PASS');await note(page,'UNSAVED-PARK-REVIEW');
 await switchTo('企业经营数据','验收归档');await expect(page.getByRole('heading',{name:/验收档案 /})).toHaveText(archiveTitle);await expect(page.getByText('冻结签收：',{exact:false})).toContainText('ENTERPRISE-RECEIPT');await expect(page.getByText('PARK-RECEIPT',{exact:false})).toHaveCount(0);
 await click(page,'返回对应验收与办理历史');await actor(page,'reviewer');await note(page,'仅作废企业空间决定');await click(page,'追加决定作废处置');await click(page,'查看已归档记录');
 await expect(page.getByRole('heading',{name:/验收档案 /})).toHaveText(archiveTitle);await expect(page.getByText('原决定已作废',{exact:true})).toBeVisible();
 await switchTo('园区能耗分析','业务验收');await expect(page.getByText(/验收单 1 · 修订.*待验收/)).toBeVisible();await expect(page.getByRole('textbox',{name:'办理说明',exact:true})).toHaveValue('');await expect(page.getByRole('combobox',{name:'核验 字段与约定一致',exact:true})).toHaveValue('');
 await submissionRows(page).locator('summary').click();await expect(submissionRows(page)).toContainText('PARK-RECEIPT');await expect(submissionRows(page)).not.toContainText('ENTERPRISE-RECEIPT');await expect(page.getByText('仅作废企业空间决定',{exact:false})).toHaveCount(0);
 await page.reload();await expect(page.getByText(/验收单 1 · 修订.*待验收/)).toBeVisible();await expect(page.getByRole('combobox',{name:"办理身份（预览）",exact:true})).toHaveValue('reviewer');await expect(page.getByText('仅作废企业空间决定',{exact:false})).toHaveCount(0);
});
