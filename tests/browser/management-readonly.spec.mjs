import {test,expect} from '@playwright/test';
test('modern settings uses real-source setup states without synthetic people or write controls',async({page})=>{
 const writes=[],apiReads=[];
 await page.route('**/api/**',route=>{const req=route.request();if(req.method()!=='GET'){writes.push(req.method());return route.abort()}apiReads.push(new URL(req.url()).pathname);return route.continue()});
 await page.goto('/');await page.getByRole('button',{name:'系统设置',exact:true}).click();await expect(page).toHaveURL('/settings');
 await expect(page.getByRole('heading',{name:'设置总览',exact:true})).toBeVisible();
 for(const [path,name]of [['members','空间成员'],['roles','角色权限'],['audit','审计记录'],['users','用户目录'],['organizations','组织机构'],['workspaces','工作空间'],['logs','运行日志'],['configuration','基础配置']]){
  await page.getByRole('navigation',{name:'平台管理',exact:true}).getByRole('link',{name,exact:true}).click();await expect(page).toHaveURL('/settings/'+path);await expect(page.getByRole('heading',{name:path==='logs'?'运行日志尚未接入':'真实来源未配置',exact:true})).toBeVisible();
  expect(await page.getByRole('button',{name:/保存|新增|启用|停用|授权/}).count()).toBe(0);expect(await page.getByRole('table').count()).toBe(0);
 }
 await page.setViewportSize({width:390,height:844});await page.goto('/settings/members');expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 expect(writes).toEqual([]);expect(apiReads).toEqual([]);
});
