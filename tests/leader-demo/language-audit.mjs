import {readFile,writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {parseArgs} from 'node:util';
import assert from 'node:assert/strict';
import {createReplayOutput} from './replay-output.mjs';
import {ids} from '../../tests/browser/fixture-server.mjs';
import {taskId} from '../../tests/leader-demo/readonly-fixture.mjs';
const require=createRequire(new URL('../../tests/browser/package.json',import.meta.url));const {chromium,expect}=require('@playwright/test');const ts=createRequire(new URL('../../apps/web/package.json',import.meta.url))('typescript');
const {values}=parseArgs({options:{output:{type:'string'}},strict:true});
const phase='reviewed',out=await createReplayOutput(fileURLToPath(new URL('../../',import.meta.url)),values.output);console.log('Audit directory: '+fileURLToPath(out));
async function keys(file,name){const s=await readFile(new URL('../../'+file,import.meta.url),'utf8'),sf=ts.createSourceFile(file,s,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);let found;function visit(n){if(ts.isVariableDeclaration(n)&&n.name.getText(sf)===name)found=n.initializer;ts.forEachChild(n,visit);}visit(sf);return found.properties.map(p=>p.name.text??p.name.getText(sf));}
const local=[...new Set(['home','develop','ingestion','processing','manage','assets','metadata','quality-list','asset','quality','publish',...await keys('apps/web/components/governance-demo/SectionPage.jsx','sectionDefs'),...await keys('apps/web/components/governance-demo/AdminPages.jsx','adminDefs')])].filter(k=>!k.startsWith('agent-'));
const pages=[...local.map(key=>({key,path:'/#'+key})),...[
 ['agent','/agent'],['agent-task','/agent/tasks?taskId='+taskId],['agent-repair','/agent/tasks/repair?taskId='+taskId],['agent-quality',`/agent/quality?versionId=${ids.goldVersion}&assessmentId=${ids.goldAssessment}`],['core-assets','/core/assets'],['core-version',`/core/assets?datasetId=${ids.goldDataset}&versionId=${ids.goldVersion}`],['core-workbench','/core/workbench'],['core-releases','/core/releases'],['core-certifications',`/core/certifications?datasetId=${ids.goldDataset}&versionId=${ids.goldVersion}`],['core-deliveries','/core/deliveries'],['core-archives','/core/archives'],['core-acceptance','/core/acceptance-context'],['core-trace','/core/release-trace']
].map(([key,path])=>({key,path}))];
const browser=await chromium.launch({channel:'msedge',headless:true}),errors=[],writes=[],entries=[];
try{
 const context=await browser.newContext({viewport:{width:1600,height:1000},deviceScaleFactor:1});
 await context.route('**/*',route=>{const r=route.request(),u=new URL(r.url());if(!['localhost','127.0.0.1'].includes(u.hostname)&&u.protocol!=='data:')return route.abort();if(u.pathname.startsWith('/api/')&&r.method()!=='GET'){writes.push(r.method()+' '+u.pathname);return route.abort();}return route.continue();});
 const p=await context.newPage();p.on('pageerror',e=>errors.push(e.message));
 const health=await p.request.get('http://127.0.0.1:4493/__health');assert.equal(health.status(),200);assert.deepEqual(await health.json(),{source:'synthetic-readonly',businessWrites:false,models:false,dispatch:false});
 for(let i=0;i<pages.length;i++){
  const {key,path}=pages[i];await p.goto('http://127.0.0.1:4193'+path);await p.locator('h1').first().waitFor();await p.waitForLoadState('networkidle');await expect(p.locator('[data-testid$="-pending"]')).toHaveCount(0);if(key==='agent'||key==='agent-repair')await expect(p.getByRole('button',{name:'保存并冻结计划',exact:true})).toBeDisabled();if(key==='agent-task')await expect(p.getByRole('heading',{name:'质量问题修复',exact:true})).toBeVisible();await p.evaluate(async()=>{await document.fonts.ready;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));});
  const file=String(i+1).padStart(2,'0')+'-'+key+'.png';await p.screenshot({path:fileURLToPath(new URL(file,out)),fullPage:true});
  const data=await p.evaluate(()=>({heading:document.querySelector('h1')?.textContent,text:document.querySelector('main')?.innerText,fields:[...document.querySelectorAll('main input,main select,main textarea')].map(e=>({label:e.getAttribute('aria-label'),placeholder:e.getAttribute('placeholder'),value:e.value})),links:[...document.querySelectorAll('main a')].map(e=>({text:e.innerText,href:e.getAttribute('href')}))}));entries.push({step:i+1,key,path,url:p.url(),screenshot:file,...data});console.log(`${i+1}/${pages.length} ${key}: ${data.heading}`);
 }
 assert.deepEqual(errors,[]);assert.deepEqual(writes,[]);
 await writeFile(new URL('pages.json',out),JSON.stringify({phase,pages:entries,errors,writes},null,2));console.log(JSON.stringify({pages:entries.length,errors,writes}));
}finally{await browser.close();}
