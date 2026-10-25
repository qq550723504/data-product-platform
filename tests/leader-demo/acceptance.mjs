// Replays the agreed demo in a fresh Edge context. No business commands.
import {createRequire} from 'node:module';
import {writeFile,readFile,readdir} from 'node:fs/promises';
import {parseArgs} from 'node:util';
import {createReplayOutput} from './replay-output.mjs';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
import {ids} from '../browser/fixture-server.mjs';
import {taskId,diagnosisId,repairId} from './readonly-fixture.mjs';
const require=createRequire(new URL('../browser/package.json',import.meta.url));
const {chromium,expect}=require('@playwright/test');
const base='http://127.0.0.1:4193';
const {values}=parseArgs({options:{output:{type:'string'}},strict:true});
const output=await createReplayOutput(fileURLToPath(new URL('../../',import.meta.url)),values.output);
console.log('Evidence directory: '+fileURLToPath(output));
const sourcePaths=['apps/web/app/layout.tsx','apps/web/app/globals.css','apps/web/components/shell.tsx','apps/web/components/governance-demo/App.jsx','apps/web/components/governance-demo/DemoState.jsx','apps/web/components/governance-demo/WorkspaceContext.jsx','apps/web/components/governance-demo/BusinessAcceptancePages.jsx','apps/web/app/agent/tasks/workbench.tsx','tests/leader-demo/readonly-fixture.mjs','tests/browser/fixture-server.mjs','tests/leader-demo/preview.ps1','tests/leader-demo/acceptance.mjs','tests/leader-demo/replay-output.mjs'];
const sourceHashes={};for(const path of sourcePaths)sourceHashes[path]=createHash('sha256').update(await readFile(new URL('../../'+path,import.meta.url))).digest('hex');
const worktreeHead=execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim();
const browser=await chromium.launch({channel:'msedge',headless:true});
const results=[],errors=[],writes=[],external=[];
let page;
async function fresh(storageFailure=false){
 const context=await browser.newContext({viewport:{width:1600,height:1000},deviceScaleFactor:1});
 await context.route('**/*',route=>{const req=route.request(),url=new URL(req.url());if(!['data:','blob:'].includes(url.protocol)&&url.origin!==base){external.push(url.origin);return route.abort();}if(url.pathname.startsWith('/api/')&&!['GET','HEAD'].includes(req.method())){writes.push({method:req.method(),path:url.pathname});return route.abort();}return route.continue();});
 if(storageFailure)await context.addInitScript(()=>{Storage.prototype.setItem=function(){throw new DOMException('Synthetic storage refusal','QuotaExceededError')};});
 const p=await context.newPage();p.on('pageerror',e=>errors.push(e.message));return p;
}
const click=name=>page.getByRole('button',{name,exact:true}).click();
const actor=value=>page.getByRole('combobox',{name:'演示办理身份',exact:true}).selectOption(value);
const note=value=>page.getByRole('textbox',{name:'办理说明',exact:true}).fill(value);
const rows=()=>page.getByRole('row').filter({hasText:/DEMO-SUB-/});
async function capture(name){
 await page.evaluate(async()=>{await document.fonts.ready;if(document.activeElement instanceof HTMLElement)document.activeElement.blur();window.scrollTo({top:0,left:0,behavior:"instant"});await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
 await page.mouse.move(1598,2);await page.screenshot({path:fileURLToPath(new URL(name+'.png',output)),fullPage:true});
 if(name==='03-quality-report'){await page.getByRole('heading',{name:'评测报告',exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:fileURLToPath(new URL('03-quality-focus.png',output)),fullPage:false});}
}
async function step(name,fn,shot){try{await fn();if(shot)await capture(shot);results.push({step:results.length+1,name,status:'PASS',url:page.url(),...(shot?{screenshot:shot+'.png'}:{})});console.log('PASS '+name);}catch(e){await capture('FAILED-'+(results.length+1)).catch(()=>{});results.push({step:results.length+1,name,status:'FAIL',error:String(e).slice(0,1600),url:page.url()});throw e;}}
async function receipt(evidence,outcome='RECEIVED'){
 await click('核对签收记录');await actor('receiver');
 await page.getByRole('combobox',{name:'收到情况',exact:true}).selectOption(outcome);
 await page.getByRole('textbox',{name:'签收依据',exact:true}).fill(evidence);
 await page.getByRole('textbox',{name:'签收说明',exact:true}).fill('合成材料逐项核对：'+evidence);
 await click('保存签收记录');await click('进入业务验收');
}
async function submit(){await actor('provider');await click('确认冻结并提交验收');}
async function pass(){await actor('reviewer');for(const n of ['字段与约定一致','统计期间符合需求','成果满足业务使用要求'])await page.getByRole('combobox',{name:'核验 '+n,exact:true}).selectOption('PASS');}
async function switchTo(name,menu){await page.getByRole('button',{name:/^切换工作空间，当前：/}).click();await page.getByRole('button',{name:new RegExp('^'+name)}).click();await expect(page.locator('.workspace-scope-bar strong')).toHaveText(name);await click('数据开发中心');await click(menu);}
try{
 page=await fresh();
 const health=await page.request.get('http://127.0.0.1:4493/__health');assert.equal(health.status(),200);assert.deepEqual(await health.json(),{source:'synthetic-readonly',businessWrites:false,models:false,dispatch:false});
 await step('首页统一新版与演示边界',async()=>{await page.goto(base);await expect(page.getByRole('heading',{name:'数据有序，成果可见'})).toBeVisible();await expect(page.locator('.integration-boundary')).toContainText('真实来源未配置');await expect(page.locator('body')).not.toContainText('沿同一成果版本推进治理与业务验收');await page.getByRole('button',{name:'原型说明',exact:true}).click();await expect(page.getByRole('dialog')).toContainText('未连接数据库或既有 Agent 服务');await expect(page.getByRole('dialog')).toContainText('计划确认与动作意图写入关闭');await capture('00-about-boundary');await page.getByRole('button',{name:'关闭弹窗',exact:true}).click();},'01-home');
 await step('Agent 工作台：原控制器与关闭的保存入口',async()=>{await click('Agent 工作台');await expect(page.getByRole('heading',{name:'让目标，成为下一步'})).toBeVisible();await expect(page.getByRole('button',{name:'保存并冻结计划',exact:true})).toBeDisabled();await expect(page.locator('.integration-boundary')).toContainText('合成只读响应');await page.getByRole('link',{name:/审阅修复计划/}).click();await expect(page.locator('#agent-recent')).toBeInViewport();assert.equal(new URL(page.url()).hash,'#agent-recent');await page.evaluate(()=>window.scrollTo({top:0,behavior:'instant'}));},'02-agent');
 await step('Agent 场景卡进入目录，选择明确版本',async()=>{await page.getByRole('link',{name:/诊断一个质量问题/}).click();await page.getByRole('link',{name:'查看版本 →',exact:true}).click();await page.getByRole('link',{name:'核实上下文 →',exact:true}).click();await expect(page.getByRole('heading',{name:'评测历史',exact:true})).toBeVisible();});
 await step('可见评测均可打开，明确失败报告与依据一致',async()=>{await expect(page.locator('a.agent-history-row')).toHaveCount(2);await page.locator(`a.agent-history-row[href*="assessmentId=${ids.goldAssessment}"]`).click();await expect(page.getByRole('heading',{name:'评测报告',exact:true})).toBeVisible();assert.equal(new URL(page.url()).searchParams.get('assessmentId'),ids.goldAssessment);await expect(page.locator('body')).not.toContainText('UNKNOWN · 无法确认');await expect(page.getByText('negative-energy',{exact:true})).toBeVisible();assert.notEqual(await page.locator('.badge-bad').first().evaluate(el=>getComputedStyle(el).color),await page.locator('.badge-good').first().evaluate(el=>getComputedStyle(el).color));await page.locator('summary').filter({hasText:'78787878-7878-4878-8878-787878787878'}).click();},'03-quality-report');
 await step('旧评测独立可读，刷新不替换为当前失败报告',async()=>{await page.locator(`a.agent-history-row[href*="assessmentId=${ids.qualityAssessment}"]`).click();await page.waitForURL(`**assessmentId=${ids.qualityAssessment}`);await expect(page.locator('body')).not.toContainText('UNKNOWN · 无法确认');await page.reload();assert.equal(new URL(page.url()).searchParams.get('assessmentId'),ids.qualityAssessment);await expect(page.locator('.agent-history-row[aria-current="true"]')).toContainText('PASS');});
 await step('Agent 任务详情、冻结修复范围与人工边界',async()=>{await page.getByRole('link',{name:/Agent 工作台$/}).click();await page.getByRole('button',{name:new RegExp('质量治理委托.*'+taskId)}).click();await expect(page.getByRole('heading',{name:'质量问题修复',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'确认当前计划',exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'保存动作意图',exact:true})).toBeDisabled();await expect(page.getByRole('heading',{name:'任务上下文',exact:true})).toBeVisible();},'04-agent-repair');
 await step('任务来源回跳保留版本及评测',async()=>{await page.getByRole('link',{name:'查看来源质量上下文 →',exact:true}).click();await page.waitForURL('**/agent/quality?**');const q=new URL(page.url()).searchParams;assert.equal(q.get('versionId'),ids.goldVersion);assert.equal(q.get('assessmentId'),ids.goldAssessment);await expect(page.getByRole('heading',{name:'评测报告',exact:true})).toBeVisible();await page.goBack();await expect(page.getByRole('heading',{name:'质量问题修复',exact:true})).toBeVisible();});
 await step('历史计划只读、任务刷新恢复及修复编辑页',async()=>{await page.getByRole('button',{name:'V1 · DIAGNOSIS',exact:true}).click();await expect(page.getByText('当前查看历史计划，只用于追溯，不能办理。',{exact:true})).toBeVisible();await page.reload();assert.equal(new URL(page.url()).searchParams.get('planId'),diagnosisId);await page.getByRole('button',{name:'V2 · REPAIR',exact:true}).click();await page.getByRole('tab',{name:'意图与历史',exact:true}).click();await expect(page.getByText('暂无动作意图',{exact:true})).toBeVisible();await page.goto(base+`/agent/tasks/repair?taskId=${taskId}`);await expect(page.getByRole('heading',{name:'配置修复计划',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'保存并冻结计划',exact:true})).toBeDisabled();},'05-repair-readonly');
 await step('缺失质量对象显示 UNKNOWN，不回退合成成功',async()=>{await page.goto(base+`/agent/quality?versionId=${ids.goldVersion}&assessmentId=${taskId}`);await expect(page.getByText('UNKNOWN · 无法确认',{exact:true}).first()).toBeVisible();},'06-unknown');
 await step('API/BFF 写拒绝，跨空间对象与控制入口不可访问',async()=>{const bff=await page.request.post(base+'/api/agent-tasks',{data:{}});assert.equal(bff.status(),403);const core=await page.request.post('http://127.0.0.1:4493/api/v1/workspaces/'+ids.workspace+'/agent-tasks',{data:{}});assert.equal(core.status(),405);const foreign=await page.request.get('http://127.0.0.1:4493/api/v1/workspaces/'+taskId+'/agent-tasks',{headers:{Authorization:'Bearer review-secret'}});assert.equal(foreign.status(),404);const control=await page.request.get('http://127.0.0.1:4493/__control/state');assert.equal(control.status(),404);});
 page=await fresh();
 await step('业务验收缺少签收不可提交；非接收人不可签收',async()=>{await page.goto(base+'/#business-acceptances');await click('查看验收');await expect(page.getByRole('button',{name:'确认冻结并提交验收',exact:true})).toBeDisabled();await click('核对签收记录');await page.getByRole('textbox',{name:'签收依据',exact:true}).fill('未授权角色测试');await page.getByRole('textbox',{name:'签收说明',exact:true}).fill('角色隔离');await expect(page.getByRole('button',{name:'保存签收记录',exact:true})).toBeDisabled();await click('进入业务验收');},'07-receipt-required');
 await step('部分收到阻断；补正追加完整签收',async()=>{await receipt('DEMO-LEADER-PARTIAL','PARTIAL');await expect(page.getByRole('button',{name:'确认冻结并提交验收',exact:true})).toBeDisabled();await receipt('DEMO-LEADER-COMPLETE');await click('核对签收记录');await expect(page.getByRole('row').filter({hasText:/DEMO-RCT-/})).toHaveCount(2);},'08-receipt');
 await step('冻结提交与责任分离；未核验不能通过',async()=>{await click('进入业务验收');await submit();await expect(page.getByRole('button',{name:'确认验收通过',exact:true})).toBeDisabled();await actor('reviewer');await expect(page.getByRole('button',{name:'确认验收通过',exact:true})).toBeDisabled();await page.getByRole('combobox',{name:'核验 统计期间符合需求',exact:true}).selectOption('FAIL');await note('补充统计期间核对材料');await click('退回修改');await expect(rows()).toContainText('已退回');});
 await step('重新提交与逐项人工通过，原退回历史保留',async()=>{await submit();await pass();await note('合成材料逐项核对通过；仅为界面演示');await click('确认验收通过');await expect(rows()).toHaveCount(2);await expect(page.getByText(/修订.*验收通过.*未归档/)).toBeVisible();},'09-acceptance');
 await step('独立归档与刷新恢复同一档案',async()=>{await actor('archivist');await click('冻结验收档案');const h=page.getByRole('heading',{name:/验收档案 · DEMO-ARC-/});const id=await h.innerText();await page.reload();await expect(h).toHaveText(id);await expect(page.getByText('冻结签收：',{exact:false})).toContainText('DEMO-LEADER-COMPLETE');},'10-archive');
 await step('空间切换不显示原空间档案，切回恢复',async()=>{await switchTo('园区能耗分析','验收归档');await expect(page.getByText(/尚无验收档案/)).toBeVisible();await expect(page.getByText('DEMO-LEADER-COMPLETE',{exact:false})).toHaveCount(0);await switchTo('企业经营数据','验收归档');await expect(page.getByRole('heading',{name:/验收档案 · DEMO-ARC-/})).toBeVisible();});
 await step('决定作废追加处置，不覆盖原通过与归档',async()=>{await click('返回对应验收与办理历史');await actor('reviewer');await note('合成异常：原核验依据有误，追加作废');await click('追加决定作废处置');await click('查看已归档记录');await expect(page.getByText('原决定已作废',{exact:true})).toBeVisible();await expect(page.getByText(/原决定：.*通过/)).toBeVisible();await page.reload();await expect(page.getByText('原决定已作废',{exact:true})).toBeVisible();},'11-invalidation');
 await step('历史追溯锁定归档签收与明确提交',async()=>{await click('追溯归档引用的签收');const r=page.getByRole('row').filter({hasText:'归档引用'});await expect(r).toContainText('DEMO-LEADER-COMPLETE');await expect(r).not.toContainText('DEMO-LEADER-PARTIAL');await click('进入业务验收');await expect(rows().filter({hasText:'第2次'}).locator('details')).toHaveAttribute('open','');},'12-trace');
 page=await fresh();
 await step('撤回、重新提交、取消保留两次冻结材料',async()=>{await page.goto(base+'/#business-acceptances');await click('查看验收');await receipt('DEMO-WITHDRAW');await submit();await actor('reviewer');await note('验收人不能撤回');await expect(page.getByRole('button',{name:'撤回提交',exact:true})).toBeDisabled();await actor('provider');await note('补充材料后再提交');await click('撤回提交');await submit();await note('第二次撤回');await click('撤回提交');await note('合成业务取消');await click('取消验收');await page.reload();await expect(rows()).toHaveCount(2);await expect(page.getByText(/修订.*已取消/)).toBeVisible();await expect(page.getByRole('button',{name:'确认冻结并提交验收',exact:true})).toHaveCount(0);});
 page=await fresh();
 await step('签收依据变化阻断通过；退回与补正不覆盖旧材料',async()=>{await page.goto(base+'/#business-acceptances');await click('查看验收');await receipt('FROZEN-A');await submit();await receipt('CORRECTED-B');await pass();await note('依据变化，必须重新提交');await expect(page.getByRole('button',{name:'确认验收通过',exact:true})).toBeDisabled();await click('退回修改');await rows().first().locator('summary').click();await expect(rows().first()).toContainText('FROZEN-A');await expect(rows().first()).not.toContainText('CORRECTED-B');await submit();await expect(rows()).toHaveCount(2);},'13-correction');
 page=await fresh(true);
 await step('会话存储写入失败明确警告，不能声称刷新已保存',async()=>{await page.goto(base+'/#business-receipts');await expect(page.getByRole('alert').filter({hasText:'本地会话存储不可用'})).toBeVisible();await actor('receiver');await page.getByRole('textbox',{name:'签收依据',exact:true}).fill('STORAGE-UNAVAILABLE');await page.getByRole('textbox',{name:'签收说明',exact:true}).fill('合成存储故障');await click('保存签收记录');await expect(page.getByText('演示记录已更新：签收登记',{exact:true})).toBeVisible();await expect(page.getByText('演示记录已保存：签收登记',{exact:true})).toHaveCount(0);await capture('14-storage-unavailable');await page.reload();await expect(page.getByRole('row').filter({hasText:'STORAGE-UNAVAILABLE'})).toHaveCount(0);});
 assert.deepEqual(errors,[],'No uncaught page errors');assert.deepEqual(writes,[],'No browser business/API writes');assert.deepEqual(external,[],'No external services');
}catch(e){console.error(String(e));process.exitCode=1;}finally{
 const screenshots={};for(const path of (await readdir(output)).filter(p=>p.endsWith('.png')))screenshots[path]=createHash('sha256').update(await readFile(new URL(path,output))).digest('hex');
 const sourcesUnchanged=await Promise.all(sourcePaths.map(async path=>createHash('sha256').update(await readFile(new URL('../../'+path,import.meta.url))).digest('hex')===sourceHashes[path]));
 if(sourcesUnchanged.includes(false))process.exitCode=1;
 await writeFile(new URL('acceptance.json',output),JSON.stringify({recordedAt:new Date().toISOString(),base,worktreeReference:{head:worktreeHead,sourceHashes,scope:'WORKTREE_AT_CAPTURE_START_NOT_RUNTIME_BUILD_ATTESTATION',unchangedDuringCapture:!sourcesUnchanged.includes(false)},screenshots,browser:'Microsoft Edge via Playwright (fresh contexts)',source:'synthetic GET-only fixture and per-tab local demo',realSources:'UNCONFIGURED',businessWrites:'DISABLED',model:'NOT_RUN',dispatch:'NOT_RUN',errors,writes,external,verdict:process.exitCode?'FAIL':'PASS',results},null,2)+'\n');
 await browser.close();
}
