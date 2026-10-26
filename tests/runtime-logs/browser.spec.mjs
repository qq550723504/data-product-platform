import {test,expect} from '../browser/node_modules/@playwright/test/index.mjs';
import {mkdirSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {startLogHost} from './host.mjs';
let host,errors,external;
const rows=page=>page.getByRole('table',{name:'运行日志',exact:true}).locator('tbody tr');
const shot=async(page,name)=>{const d=new URL('../../.data/runtime-logs-20261026/screenshots/',import.meta.url);mkdirSync(d,{recursive:true});await page.screenshot({path:fileURLToPath(new URL(name+'.png',d)),fullPage:true})};
test.beforeEach(async({page})=>{
 host=await startLogHost();errors=[];external=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/*',route=>{if(new URL(route.request().url()).origin!==host.origin){external.push('outside isolated origin');return route.abort()}return route.continue()});
 await page.goto(host.origin+'/_login');await expect(rows(page)).toHaveCount(25);
});
test.afterEach(async()=>{try{expect(errors).toEqual([]);expect(external).toEqual([]);expect(host.errors).toEqual([]);expect(host.requests.filter(r=>!['GET','HEAD'].includes(r.method))).toEqual([])}finally{await host.close()}});
test('RL01: unconfigured source shows one setup state without synthetic records or queries',async({page})=>{
 host.source.state.source='UNCONFIGURED';const calls=host.source.state.calls.length;await page.goto(host.origin+'/settings/logs?mode=synthetic');await expect(page.getByRole('heading',{name:'运行日志尚未接入',exact:true})).toBeVisible();expect(await page.getByRole('table').count()).toBe(0);expect(host.source.state.calls.length).toBe(calls);await shot(page,'unconfigured');
});
test('RL06: all 58 independent records including 28 equal nanoseconds survive three pages',async({page})=>{
 const ids=[];for(const count of [25,25,8]){await expect(rows(page)).toHaveCount(count);ids.push(...await rows(page).evaluateAll(rs=>rs.map(r=>r.id)));if(count===25)await page.getByRole('link',{name:'更早记录',exact:true}).click()}
 expect(ids).toHaveLength(58);expect(new Set(ids).size).toBe(58);expect(await page.getByRole('link',{name:'更早记录',exact:true}).count()).toBe(0);
});
test('RL09: second page detail refresh history and return restore fixed query and focused row',async({page})=>{
 await page.getByRole('link',{name:'更早记录',exact:true}).click();const list=page.url(),id=await rows(page).last().getAttribute('id');await shot(page,'list');await rows(page).last().getByRole('link',{name:'查看记录',exact:true}).click();const detail=page.url();await expect(page.getByRole('region',{name:'运行记录详情',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);await page.reload();expect(page.url()).toBe(detail);await shot(page,'detail');await page.goBack();await expect(rows(page)).toHaveCount(25);expect(page.url()).toBe(list);await page.goForward();await expect(page.getByRole('region',{name:'运行记录详情',exact:true})).toBeVisible();
 await page.getByRole('link',{name:'返回运行日志',exact:true}).click();expect(new URL(page.url()).searchParams.get('cursor')).toBe(new URL(list).searchParams.get('cursor'));await expect(page.locator('tr:target')).toHaveAttribute('id',id);await expect(page.locator('tr:target')).toBeFocused();await page.reload();await expect(page.locator('tr:target')).toBeFocused();expect(new URL(page.url()).searchParams.has('entryRef')).toBe(false);await shot(page,'return');
});
test('RL10: filters clear old positions while reload preserves explicit UTC window',async({page})=>{
 await page.getByRole('link',{name:'更早记录',exact:true}).click();const start=new URL(page.url()).searchParams.get('start'),end=new URL(page.url()).searchParams.get('end');await page.getByRole('combobox',{name:'服务',exact:true}).selectOption('APP_SERVICE');await page.getByRole('button',{name:'查询',exact:true}).click();expect(new URL(page.url()).searchParams.has('cursor')).toBe(false);await expect(rows(page)).toHaveCount(25);await expect(rows(page).first()).toContainText('应用服务');await page.reload();expect(new URL(page.url()).searchParams.get('start')).toBe(start);expect(new URL(page.url()).searchParams.get('end')).toBe(end);await page.getByRole('combobox',{name:'级别',exact:true}).selectOption('WARN');await page.getByRole('button',{name:'查询',exact:true}).click();await expect(rows(page)).toHaveCount(10);await page.getByRole('link',{name:'清除条件',exact:true}).click();await expect(rows(page)).toHaveCount(25);expect(new URL(page.url()).searchParams.has('service')).toBe(false);
});
test('RL03 RL13: switching workspace rejects copied cursor and detail without foreign data',async({page})=>{
 const next=await page.getByRole('link',{name:'更早记录',exact:true}).getAttribute('href'),detail=await rows(page).first().getByRole('link',{name:'查看记录',exact:true}).getAttribute('href');await page.goto(host.origin+'/_login?space=B');await expect(page.getByText('当前工作空间 · 合成空间乙',{exact:true})).toBeVisible();for(const path of [next,detail]){await page.goto(host.origin+path);await expect(page.getByRole('heading',{name:'没有当前空间的运行日志查看权限',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);expect(await page.getByRole('region',{name:'运行记录详情',exact:true}).count()).toBe(0)}
});
test('RL11: another person cannot reuse an otherwise valid same-space reference',async({page})=>{
 const url=await rows(page).first().getByRole('link',{name:'查看记录',exact:true}).getAttribute('href');await page.goto(host.origin+'/_login?person=two');await page.goto(host.origin+url);await expect(page.getByRole('heading',{name:'没有当前空间的运行日志查看权限',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);
});
test('RL02 RL04: log capability removal during query discards the entire response',async({page})=>{
 host.source.state.afterRead=async()=>{host.source.state.logRead=false};await page.reload();await expect(page.getByRole('heading',{name:'没有当前空间的运行日志查看权限',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);expect(await page.locator('[data-runtime-log-results]').count()).toBe(0);await shot(page,'revoked');
});
test('RL04: RP revocation or revision changes block refresh back and forward without reviving cached records',async({page})=>{
 await rows(page).first().getByRole('link',{name:'查看记录',exact:true}).click();host.fixtures.A.state.deny=true;await page.goBack();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);await page.goForward();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();host.fixtures.A.state.deny=false;host.fixtures.A.state.revision='1:1:2';await page.reload();await expect(page.getByRole('heading',{name:'登录状态不可用',exact:true})).toBeVisible();
});
test('RL05: source binding changes invalidate old cursor without silently returning first page',async({page})=>{
 const next=await page.getByRole('link',{name:'更早记录',exact:true}).getAttribute('href');host.source.state.version++;await page.goto(host.origin+next);await expect(page.getByRole('heading',{name:'查询位置已失效',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0);await page.getByRole('link',{name:'按当前条件重新查询',exact:true}).click();await expect(rows(page)).toHaveCount(25);expect(new URL(page.url()).searchParams.has('cursor')).toBe(false);
});
test('RL11: expired references require explicit restart and preserve filters',async({page})=>{
 await page.getByRole('combobox',{name:'服务',exact:true}).selectOption('APP_SERVICE');await page.getByRole('button',{name:'查询',exact:true}).click();const next=await page.getByRole('link',{name:'更早记录',exact:true}).getAttribute('href');for(const ref of host.source.references.values())ref.expires=0;await page.goto(host.origin+next);await expect(page.getByRole('heading',{name:'查询位置已失效',exact:true})).toBeVisible();await expect(page.getByRole('combobox',{name:'服务',exact:true})).toHaveValue('APP_SERVICE');expect(await rows(page).count()).toBe(0);
});
test('RL07 RL08: malformed or private source fields never reach HTML DOM or storage',async({page})=>{
 host.source.state.fault=data=>{data.items[0].body='PRIVATE_LOG_BODY';data.items[0].stack='PRIVATE_STACK'};const response=await page.reload();await expect(page.getByRole('heading',{name:'运行记录暂时无法读取',exact:true})).toBeVisible();expect(await response.text()).not.toMatch(/PRIVATE_LOG_BODY|PRIVATE_STACK/);expect(await page.content()).not.toMatch(/PRIVATE_LOG_BODY|PRIVATE_STACK/);expect(await page.evaluate(()=>localStorage.length+sessionStorage.length)).toBe(0);expect(await rows(page).count()).toBe(0);
});
test('RL12: deleted precise record returns to the original page and reports missing focused row',async({page})=>{
 await page.getByRole('link',{name:'更早记录',exact:true}).click();const link=await rows(page).last().getByRole('link',{name:'查看记录',exact:true}).getAttribute('href'),key=new URL(link,host.origin).searchParams.get('focus'),cursor=new URL(link,host.origin).searchParams.get('cursor');host.source.state.records=host.source.state.records.filter(r=>host.source.rowKey(r)!==key);await page.goto(host.origin+link);await expect(page.getByRole('heading',{name:'指定运行记录当前不可见',exact:true})).toBeVisible();await page.getByRole('link',{name:'返回运行日志',exact:true}).click();expect(new URL(page.url()).searchParams.get('cursor')).toBe(cursor);await expect(page.getByText('原记录当前不可见',{exact:true})).toBeVisible();
});
test('RL12: a changed projection cannot masquerade as the original detail',async({page})=>{
 const link=await rows(page).first().getByRole('link',{name:'查看记录',exact:true}).getAttribute('href'),key=new URL(link,host.origin).searchParams.get('focus');host.source.state.records.find(r=>host.source.rowKey(r)===key).level='FATAL';await page.goto(host.origin+link);await expect(page.getByRole('heading',{name:'运行记录已变化，请重新查询',exact:true})).toBeVisible();expect(await page.getByRole('region',{name:'运行记录详情',exact:true}).count()).toBe(0);
});
test('RL12: no matches differ from source failure and unsupported queries',async({page})=>{
 host.source.state.records=[];await page.reload();await expect(page.getByText('当前条件下没有运行记录',{exact:true})).toBeVisible();host.source.state.fault=()=>{throw Error('PRIVATE_PROVIDER_FAILURE')};await page.reload();await expect(page.getByRole('heading',{name:'运行记录暂时无法读取',exact:true})).toBeVisible();expect(await page.content()).not.toContain('PRIVATE_PROVIDER_FAILURE');host.source.state.source='UNSUPPORTED';await page.reload();await expect(page.getByRole('heading',{name:'来源暂不支持当前查询',exact:true})).toBeVisible();
});
test('RL14: narrow layout query and return remain reachable with keyboard focus',async({page})=>{
 await page.setViewportSize({width:390,height:844});await page.reload();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await expect(page.getByRole('button',{name:'查询',exact:true})).toBeVisible();await rows(page).first().getByRole('link',{name:'查看记录',exact:true}).click();await expect(page.getByRole('link',{name:'返回运行日志',exact:true})).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await shot(page,'mobile');await page.getByRole('link',{name:'返回运行日志',exact:true}).click();await expect(page.locator('tr:target')).toBeFocused();
});
test('RL15: writes native selectors and caller-supplied identity are rejected; every response is no-store',async({page,context})=>{
 for(const method of ['POST','DELETE','PATCH']){const r=await context.request.fetch(host.origin+'/settings/logs',{method});expect(r.status()).toBe(403)}
 // These deliberate rejections are test probes, not product writes.
 host.requests.splice(0,host.requests.length);
 for(const extra of ['tenant=B','mode=synthetic','offset=25','start=x&start=y','from=https://external.invalid']){const r=await page.goto(page.url().split('?')[0]+'?'+extra);expect(r.headers()['cache-control']).toBe('private, no-store');await expect(page.getByRole('heading',{name:'查询条件无效',exact:true})).toBeVisible();expect(await rows(page).count()).toBe(0)}
 const r=await context.request.get(host.origin+'/settings/logs',{headers:{Authorization:'Bearer forged'}});expect(r.status()).toBe(403);
});
