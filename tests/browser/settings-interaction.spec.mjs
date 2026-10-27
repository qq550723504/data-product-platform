import {test,expect} from '@playwright/test';

const origin='http://127.0.0.1:4445';
const menus=[['settings','设置总览'],['admin-workspaces','工作空间管理'],['workspace-members','空间成员与角色'],['admin-users','用户管理'],['admin-orgs','组织机构'],['admin-roles','角色权限'],['admin-audit','审计日志'],['admin-logs','运行日志'],['admin-config','基础配置']];
const dialog=page=>page.getByRole('dialog');
const row=(page,name)=>page.getByRole('row').filter({hasText:name});
async function go(page,id){await page.goto('/#'+id);await expect(page.getByRole('heading',{level:1})).toBeVisible()}
async function close(page){await dialog(page).getByRole('button',{name:'关闭弹窗',exact:true}).click();await expect(dialog(page)).toHaveCount(0)}
async function save(page,name){await dialog(page).getByRole('button',{name,exact:true}).click();await expect(dialog(page)).toHaveCount(0)}

test.beforeEach(async({page})=>{
 const violations=[];const errors=[];
 page.on('request',r=>{const u=new URL(r.url());if(u.origin!==origin||!['GET','HEAD'].includes(r.method())||/^\/(api|auth|core|agent)(\/|$)/.test(u.pathname))violations.push(r.method()+' '+u.origin+u.pathname)});
 page.on('pageerror',e=>errors.push(e.message));
 await page.exposeFunction('interactionViolations',()=>({violations,errors}));
});
test.afterEach(async({page})=>{if(!page.isClosed()&&page.url()!=='about:blank'){const result=await page.evaluate(()=>window.interactionViolations());expect(result).toEqual({violations:[],errors:[]})}});

test('all settings menus, title, about and settings links stay in the local prototype',async({page})=>{
 for(const [id,title] of menus){await go(page,id);await expect(page.getByRole('heading',{name:title,exact:true,level:1})).toBeVisible();await expect(page).toHaveTitle(new RegExp('创平数据治理平台'));}
 await page.getByRole('button',{name:'系统设置',exact:true}).click();await expect(page).toHaveURL(/\/#settings$/);
 await page.getByRole('button',{name:'进入用户管理',exact:true}).click();await expect(page).toHaveURL(/#admin-users$/);
 await page.getByRole('button',{name:'系统说明',exact:true}).click();await expect(dialog(page)).toContainText('关于创平数据治理平台');await close(page);
 await page.goto('/settings/users');await expect(page).toHaveURL(origin+'/#admin-users');await expect(page.getByRole('heading',{level:1,name:'用户管理',exact:true})).toBeVisible();
 await page.screenshot({path:'.data/settings-interaction-users.png',fullPage:true});
});

test('users create, cancel, edit, disable, enable and refresh locally',async({page})=>{
 await go(page,'admin-users');await page.getByRole('button',{name:/^新增/}).click();await dialog(page).getByLabel('姓名',{exact:true}).fill('取消成员');await close(page);await expect(row(page,'取消成员')).toHaveCount(0);
 await page.getByRole('button',{name:/^新增/}).click();await dialog(page).getByLabel('姓名',{exact:true}).fill('交互验收成员');await dialog(page).getByRole('textbox').nth(1).fill('interaction.member');await save(page,'保存成员');
 await row(page,'交互验收成员').getByRole('button',{name:'编辑',exact:true}).click();await dialog(page).getByLabel('姓名',{exact:true}).fill('交互验收成员甲');await save(page,'保存成员');
 for(const action of ['停用','启用']){await row(page,'交互验收成员甲').getByRole('button',{name:action,exact:true}).click();await dialog(page).getByRole('button',{name:new RegExp('确认'+action)}).click();await expect(dialog(page)).toHaveCount(0);await expect(row(page,'交互验收成员甲')).toContainText(action);}
 await page.reload();await expect(row(page,'交互验收成员甲')).toContainText('interaction.member');
});

test('organization, role, access preview, configuration and audit have complete local flows',async({page})=>{
 await go(page,'admin-orgs');await page.getByRole('button',{name:'新增机构',exact:true}).click();await dialog(page).getByLabel('机构名称',{exact:true}).fill('交互验收组');await dialog(page).getByLabel('负责人',{exact:true}).fill('验收负责人');await save(page,'保存机构');await expect(row(page,'交互验收组')).toBeVisible();
 await row(page,'交互验收组').getByRole('button',{name:'编辑机构',exact:true}).click();await dialog(page).getByLabel('负责人',{exact:true}).fill('替换负责人');await save(page,'保存机构');await expect(row(page,'交互验收组')).toContainText('替换负责人');
 await go(page,'admin-roles');await page.getByRole('button',{name:'新增角色',exact:true}).click();await dialog(page).getByLabel('角色名称',{exact:true}).fill('交互验收角色');await dialog(page).getByRole('checkbox',{name:/报告中心/}).check();await save(page,'保存角色权限');await expect(row(page,'交互验收角色')).toContainText('允许');
 await page.getByRole('button',{name:'访问效果预览',exact:true}).click();await expect(dialog(page).getByRole('combobox',{name:'预览用户',exact:true})).toBeVisible();await dialog(page).getByRole('combobox',{name:'访问功能',exact:true}).selectOption('users');await close(page);
 await go(page,'admin-config');await page.getByRole('combobox',{name:'时区',exact:true}).selectOption('UTC');await page.getByRole('button',{name:'保存本地配置',exact:true}).click();await page.reload();await expect(page.getByRole('combobox',{name:'时区',exact:true})).toHaveValue('UTC');
 await go(page,'admin-audit');await page.getByRole('button',{name:'管理审计',exact:true}).click();await expect(row(page,'保存角色权限')).toBeVisible();await row(page,'保存角色权限').getByRole('button',{name:'审计详情',exact:true}).click();await expect(dialog(page)).toContainText('变更前');await expect(dialog(page)).toContainText('变更后');await close(page);
 await go(page,'admin-logs');await page.getByRole('button',{name:'日志详情',exact:true}).first().click();await expect(dialog(page)).toContainText('追踪编号');await close(page);
});

test('workspace and members local lifecycle, owner guard, isolation and refresh',async({page})=>{
 await go(page,'admin-workspaces');await page.getByRole('button',{name:'新建工作空间',exact:true}).click();await dialog(page).getByLabel('空间名称',{exact:true}).fill('交互验收空间');await save(page,'保存工作空间');
 await row(page,'交互验收空间').getByRole('button',{name:'编辑',exact:true}).click();await dialog(page).getByLabel('空间说明',{exact:true}).fill('仅供本地流程验证');await save(page,'保存工作空间');
 await row(page,'交互验收空间').getByRole('button',{name:'归档',exact:true}).click();await save(page,'确认归档');await expect(row(page,'交互验收空间')).toContainText('已归档');
 await row(page,'交互验收空间').getByRole('button',{name:'恢复',exact:true}).click();await save(page,'确认恢复');
 await row(page,'交互验收空间').getByRole('button',{name:'进入空间',exact:true}).click();await expect(page).toHaveURL(/#settings$/);
 await go(page,'workspace-members');await expect(page.getByRole('table').first().getByRole('row')).toHaveCount(2);
 await page.getByRole('button',{name:'移出空间',exact:true}).click();await dialog(page).getByRole('button',{name:'确认移出',exact:true}).click();await expect(dialog(page).getByRole('alert')).toBeVisible();await close(page);
 await page.getByRole('button',{name:'添加空间成员',exact:true}).click();await dialog(page).getByRole('combobox',{name:'平台用户',exact:true}).selectOption('USR-02');await save(page,'保存空间成员');await expect(row(page,'analyst.demo')).toBeVisible();
 await row(page,'analyst.demo').getByRole('button',{name:'调整角色',exact:true}).click();await dialog(page).getByRole('combobox',{name:'空间角色',exact:true}).selectOption('空间开发者');await save(page,'保存空间成员');await page.reload();await expect(row(page,'analyst.demo')).toContainText('空间开发者');
 await row(page,'analyst.demo').getByRole('button',{name:'移出空间',exact:true}).click();await save(page,'确认移出');await expect(row(page,'analyst.demo')).toHaveCount(0);
 await page.getByRole('button',{name:/^切换工作空间，当前：/}).click();await dialog(page).getByRole('button',{name:/^企业经营数据/}).click();await expect(page).toHaveURL(/#settings$/);await go(page,'workspace-members');await expect(row(page,'analyst.demo')).toBeVisible();
 await page.screenshot({path:'.data/settings-interaction-members.png',fullPage:true});
});

test('server rejects connected APIs, write methods and connected identity paths',async({request})=>{
 for(const path of ['/api/settings-admin/status','/auth/login','/core/assets','/agent'])expect((await request.get(path)).status()).toBe(403);
 expect([403,404]).toContain((await request.get('/%61pi/settings-admin/status')).status());
 expect((await request.post('/',{data:{name:'blocked'}})).status()).toBe(403);
 expect((await request.get('/unknown')).status()).toBe(404);
});

for(const viewport of [{width:1280,height:360},{width:390,height:600}])test(`workspace dialog fits ${viewport.width}x${viewport.height}`,async({page})=>{
 await page.setViewportSize(viewport);await go(page,'admin-workspaces');await page.getByRole('button',{name:/^切换工作空间，当前：/}).click();const d=dialog(page);await expect(d).toBeVisible();const box=await d.boundingBox();expect(box.y).toBeGreaterThanOrEqual(0);expect(box.y+box.height).toBeLessThanOrEqual(viewport.height);await page.keyboard.press('Escape');await expect(d).toHaveCount(0);
});
