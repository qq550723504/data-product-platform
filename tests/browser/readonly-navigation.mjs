// Full privileged GET navigation must finish before refreshing its document.
// Playwright click waits for navigation commitment, not completion. A subsequent
// CDP Page.reload can otherwise race Chromium's destination session attachment.
export async function reloadReadOnly(page){
 await page.waitForLoadState('load');
 // Resolve against the destination execution context as well as its load signal.
 await page.waitForFunction(()=>document.readyState==='complete');
 return page.reload({waitUntil:'load'});
}
