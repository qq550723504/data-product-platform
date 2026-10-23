import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
const require=createRequire(import.meta.url),ts=require('typescript'),Module=require('node:module');
const source=readFileSync(new URL('../../middleware.ts',import.meta.url),'utf8');
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;
const m=new Module(import.meta.filename);m.require=name=>name==='next/server'?{NextResponse:{next:()=>({action:'next'}),rewrite:url=>({action:'rewrite',url})}}:require(name);m._compile(compiled,import.meta.filename);
function request(path){const nextUrl=new URL(path,'http://local');nextUrl.clone=()=>new URL(nextUrl);return {nextUrl};}
test('only prototype root is rewritten; Core identities and existing server action paths are retained',()=>{
 const root=m.exports.middleware(request('/?retained=1'));assert.equal(root.action,'rewrite');assert.equal(root.url.pathname,'/demo');assert.equal(root.url.search,'?retained=1');
 for(const path of ['/agent','/agent/tasks','/core/workbench','/core/release-trace?releaseId=exact','/datasets/exact/versions/historical?assessmentId=report','/products/product/releases/release','/attention?qualityView=CLOSED','/reviews','/resources/resource?requestId=request'])assert.equal(m.exports.middleware(request(path)).action,'next',path);
});
