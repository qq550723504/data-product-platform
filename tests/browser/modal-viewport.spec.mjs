import {test,expect} from '@playwright/test';

const workspaceTrigger=page=>page.getByRole('button',{name:/^切换工作空间，当前：/});
async function insideViewport(page,locator){
 const bounds=await locator.boundingBox(),viewport=page.viewportSize();
 expect(bounds).not.toBeNull();
 expect(bounds.x).toBeGreaterThanOrEqual(0);expect(bounds.y).toBeGreaterThanOrEqual(0);
 expect(bounds.x+bounds.width).toBeLessThanOrEqual(viewport.width);
 expect(bounds.y+bounds.height).toBeLessThanOrEqual(viewport.height);
}

// The header's backdrop-filter used to make fixed dialogs only 73px tall.
// These checks exercise the actual rendered geometry, including short windows.
for(const viewport of [{width:1912,height:866},{width:1280,height:720},{width:1280,height:360},{width:390,height:600}]){
 test(`workspace selector remains usable within ${viewport.width}x${viewport.height}`,async({page})=>{
  await page.setViewportSize(viewport);
  await page.goto('/#admin-workspaces');
  await expect(page.getByRole('heading',{name:'工作空间管理',exact:true})).toBeVisible();
  await workspaceTrigger(page).click();
  const dialog=page.getByRole('dialog',{name:'切换工作空间',exact:true});
  await expect(dialog).toBeFocused();
  await insideViewport(page,dialog);
  await insideViewport(page,dialog.getByRole('heading',{name:'切换工作空间',exact:true}));
  await insideViewport(page,dialog.getByRole('button',{name:'关闭弹窗',exact:true}));
  const backdrop=await page.locator('.modal-backdrop').boundingBox();
  expect(backdrop).toEqual({x:0,y:0,width:viewport.width,height:viewport.height});
  const scrollBefore=await page.evaluate(()=>scrollY);
  await page.mouse.move(5,viewport.height-5);await page.mouse.wheel(0,500);
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  await expect.poll(()=>page.evaluate(()=>scrollY)).toBe(scrollBefore);
  // Shift+Tab from the initial dialog focus stays inside the dialog.
  const manage=dialog.getByRole('button',{name:'新建或管理工作空间',exact:true});
  await page.keyboard.press('Shift+Tab');await expect(manage).toBeFocused();
  await page.keyboard.press('Tab');await expect(dialog.getByRole('button',{name:'关闭弹窗',exact:true})).toBeFocused();
  const search=dialog.getByRole('textbox',{name:'搜索工作空间',exact:true});
  await search.fill('不存在的工作空间');await expect(dialog.getByText('没有匹配的可进入空间。',{exact:true})).toBeVisible();
  await search.fill('园区');
  const park=dialog.getByRole('button',{name:/^园区能耗分析/});
  await park.scrollIntoViewIfNeeded();await insideViewport(page,park);
  await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(workspaceTrigger(page)).toBeFocused();
  await expect.poll(()=>page.evaluate(()=>document.body.style.overflow)).not.toBe('hidden');
  // Reopening clears the search and still switches only the local workspace.
  await workspaceTrigger(page).click();await expect(search).toHaveValue('');await park.click();
  await expect(page).toHaveURL(/#home$/);await expect(workspaceTrigger(page)).toHaveAccessibleName('切换工作空间，当前：园区能耗分析');
  await workspaceTrigger(page).click();await dialog.getByRole('button',{name:/^企业经营数据/}).click();
  await expect(workspaceTrigger(page)).toHaveAccessibleName('切换工作空间，当前：企业经营数据');
 });
}

test('shared header and page dialogs keep close, focus and cancellation behavior',async({page})=>{
 await page.setViewportSize({width:1912,height:866});
 const writes=[];
 await page.route('**/api/**',route=>{
  if(!['GET','HEAD'].includes(route.request().method())){writes.push(route.request().method());return route.abort()}
  return route.continue();
 });
 await page.goto('/#admin-workspaces');
 const create=page.getByRole('button',{name:'新建工作空间',exact:true});await create.click();
 const form=page.getByRole('dialog',{name:'新建工作空间',exact:true});await insideViewport(page,form);
 await form.getByRole('textbox',{name:'空间名称',exact:true}).fill('未保存空间');
 await form.getByRole('button',{name:'关闭弹窗',exact:true}).click();await expect(create).toBeFocused();
 await expect(page.getByRole('row').filter({hasText:'未保存空间'})).toHaveCount(0);
 await page.getByRole('button',{name:'系统说明',exact:true}).click();
 const about=page.getByRole('dialog');await insideViewport(page,about);await expect(about).toBeFocused();
 await page.mouse.click(3,3);await expect(about).toHaveCount(0);await expect(page.getByRole('button',{name:'系统说明',exact:true})).toBeFocused();
 expect(writes).toEqual([]);
});
