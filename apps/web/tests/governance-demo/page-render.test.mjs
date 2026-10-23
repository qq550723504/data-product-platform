// Component render coverage, not browser acceptance. No HTTP requests or Core writes.
import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import fs from 'node:fs';
const require=createRequire(import.meta.url);
const ts=require('typescript');
require.extensions['.jsx']=(mod,file)=>mod._compile(ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{jsx:ts.JsxEmit.React,module:ts.ModuleKind.CommonJS,esModuleInterop:true,target:ts.ScriptTarget.ES2022}}).outputText,file);
const navigation=require.resolve('next/navigation');
require.cache[navigation]={id:navigation,filename:navigation,loaded:true,exports:{usePathname:()=> '/',useRouter:()=>({push(){throw Error('Navigation during render')},replace(){throw Error('Navigation during render')}})}};
const React=require('react'),{renderToStaticMarkup}=require('react-dom/server');
// The package's CJS filename is inside a type=module package. Use its native
// ESM export in this Node-only harness, as Next's bundler does for the app.
const icons=require.resolve('@phosphor-icons/react');
require.cache[icons]={id:icons,filename:icons,loaded:true,exports:await import('@phosphor-icons/react')};
const {App,ConnectedApp}=require('../../components/governance-demo/App.jsx');
const {sectionDefs,menuGroups}=require('../../components/governance-demo/SectionPage.jsx');
const {adminDefs}=require('../../components/governance-demo/AdminPages.jsx');
const routes=new Set(['home','develop','ingestion','processing','manage','assets','metadata','quality-list','asset','quality','publish',...Object.keys(sectionDefs),...Object.keys(adminDefs)]);
test('every registered demo page renders a nonempty main and its heading or Core handoff',()=>{
 for(const route of routes){globalThis.location={hash:'#'+route};const html=renderToStaticMarkup(React.createElement(App,{liveAgent:false}));
  assert.match(html,/<main[^>]*id="main"/,route);
  assert.match(html,route.startsWith('agent-')?/打开已连接 Core 的 Agent 工作台/:/<h1>/,route);
  assert.doesNotMatch(html,/原业务对象链接/,route);
 }
 assert.equal(routes.size,53);
});
test('every primary menu has a registered page',()=>{
 for(const [route] of Object.values(menuGroups).flat())assert.ok(routes.has(route),route);
});

test('Core catalog renders server facts without synthetic asset content or reset controls',()=>{
 globalThis.location={hash:'#assets'};
 const html=renderToStaticMarkup(React.createElement(App,{liveAgent:false,liveCatalog:true},React.createElement('h1',null,'CORE_FACT_SENTINEL')));
 assert.match(html,/CORE_FACT_SENTINEL/);
 assert.match(html,/Core 只读/);
 assert.doesNotMatch(html,/重置演示|当前成果 · VER-|合成数据演示/);
});

test('connected shell renders server facts without browser globals or demo providers',()=>{
 delete globalThis.location;
 const html=renderToStaticMarkup(React.createElement(ConnectedApp,{liveAgent:false},React.createElement('h1',null,'SSR_CORE_FACT_SENTINEL')));
 assert.match(html,/SSR_CORE_FACT_SENTINEL/);assert.doesNotMatch(html,/重置演示|当前成果 · VER-/);
});
