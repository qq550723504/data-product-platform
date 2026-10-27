import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
const require=createRequire(new URL('../../middleware.ts',import.meta.url)),ts=require('typescript'),Module=require('node:module');
const source=readFileSync(new URL('../../middleware.ts',import.meta.url),'utf8');
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022,esModuleInterop:true,resolveJsonModule:true}}).outputText;
class NextResponse{constructor(body,options){this.action='deny';this.status=options.status;this.body=body}static next(){return {action:'next'}}static rewrite(url){return {action:'rewrite',url}}static redirect(url){return {action:'redirect',url}}}
const m=new Module(import.meta.filename);m.require=name=>name==='next/server'?{NextResponse}:require(name);m._compile(compiled,import.meta.filename);
function request(path,method='GET',host='127.0.0.1:4445'){const nextUrl=new URL(path,'http://local');nextUrl.clone=()=>new URL(nextUrl);return {nextUrl,url:nextUrl.href,method,headers:{get:name=>name==='host'?host:null}};}
test('only prototype root is rewritten; Core identities and existing server action paths are retained',()=>{
 const root=m.exports.middleware(request('/?retained=1'));assert.equal(root.action,'rewrite');assert.equal(root.url.pathname,'/demo');assert.equal(root.url.search,'?retained=1');
 for(const path of ['/agent','/agent/tasks','/core/workbench','/core/release-trace?releaseId=exact','/datasets/exact/versions/historical?assessmentId=report','/products/product/releases/release','/attention?qualityView=CLOSED','/reviews','/resources/resource?requestId=request'])assert.equal(m.exports.middleware(request(path)).action,'next',path);
});

test('explicit acceptance identity profile exposes only identity/acceptance reads and static assets; default Core transport remains isolated',()=>{
 const before=process.env.BROWSER_IAM_MODE,profile=process.env.BROWSER_IAM_PROFILE;
 try{
  process.env.BROWSER_IAM_MODE='oidc';process.env.BROWSER_IAM_PROFILE='business-acceptance';
  const home=m.exports.middleware(request('/'));assert.equal(home.action,'redirect');assert.equal(home.url.pathname,'/identity');
  for(const path of ['/identity','/core/acceptance-context?deliveryOperationId=exact','/auth/login','/_next/static/chunks/app.js'])assert.equal(m.exports.middleware(request(path)).action,'next');
  for(const path of ['/agent','/core/workbench','/api/private','/_next/data/build/datasets.json','/_next/image?url=/datasets','/core%2Facceptance-context'])assert.equal(m.exports.middleware(request(path)).status,403,path);
  assert.equal(m.exports.middleware(request('/core/acceptance-context','POST')).status,403);
  process.env.BROWSER_IAM_MODE='invalid';assert.equal(m.exports.middleware(request('/identity')).status,503);
 }finally{if(before===undefined)delete process.env.BROWSER_IAM_MODE;else process.env.BROWSER_IAM_MODE=before;if(profile===undefined)delete process.env.BROWSER_IAM_PROFILE;else process.env.BROWSER_IAM_PROFILE=profile;}
});

test('opt-in interaction preview rejects connected identity and routes; settings redirect stays loopback',()=>{
 const before=process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO,mode=process.env.BROWSER_IAM_MODE;
 try{
  process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO='true';process.env.BROWSER_IAM_MODE='off';
  assert.equal(m.exports.middleware(request('/')).url.pathname,'/demo');
  assert.equal(m.exports.middleware(request('/settings/users')).url.href,'http://127.0.0.1:4445/#admin-users');
  assert.equal(m.exports.middleware(request('/settings/users','GET','outside.invalid')).status,403);
  for(const path of ['/api/settings-admin/status','/auth/login','/core/assets','/agent'])assert.equal(m.exports.middleware(request(path)).status,403,path);
  assert.equal(m.exports.middleware(request('/','POST')).status,403);
  assert.equal(m.exports.middleware(request('/unknown')).status,404);
  process.env.BROWSER_IAM_MODE='oidc';assert.equal(m.exports.middleware(request('/')).status,503);
 }finally{if(before===undefined)delete process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO;else process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO=before;if(mode===undefined)delete process.env.BROWSER_IAM_MODE;else process.env.BROWSER_IAM_MODE=mode;}
});
