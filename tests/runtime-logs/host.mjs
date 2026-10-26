// Independent test host renders the actual shared React screen and loader.
// It has no application API, DB connection, real logs, collector, or provider configuration.
import {createServer} from 'node:https';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
import {fixture,MemoryStore} from '../../apps/web/tests/helpers/oidc-fixture.mjs';
import {syntheticSource} from '../../apps/web/tests/helpers/runtime-log-source.mjs';
const require=createRequire(new URL('../../apps/web/package.json',import.meta.url)),React=require('react'),{renderToStaticMarkup}=require('react-dom/server');
const {RuntimeLogs}=require('../../apps/web/.runtime-log-tests/components/runtime-logs.js'),{ManagementShellView}=require('../../apps/web/.runtime-log-tests/components/management-shell-view.js'),{loadRuntimeLogs,runtimeLogBrowserGuard,LogReadError}=require('../../apps/web/.runtime-log-tests/lib/runtime-logs.js');
const icons=await Promise.all(['GearSix','Buildings','Users','Users','TreeStructure','ShieldCheck','Clock','FileText','SlidersHorizontal'].map(async name=>(await import('../../apps/web/node_modules/@phosphor-icons/react/dist/csr/'+name+'.es.js'))[name]));
const css=['../../apps/web/app/identity.css','../../apps/web/app/globals.css','../../apps/web/components/governance-demo/styles.css','../../apps/web/components/governance-demo/agent.css','../../apps/web/components/governance-demo/integration.css'].map(p=>readFileSync(new URL(p,import.meta.url),'utf8')).join('\n');
export async function startLogHost(){
 const cleaners=[],t={after:fn=>cleaners.push(fn)},source=syntheticSource(),fixtures={A:await fixture(t,new MemoryStore(),'platform-management'),B:await fixture(t,new MemoryStore(),'platform-management','44444444-4444-4444-8444-444444444444')};
 const sessions=new Map(),requests=[],errors=[];let origin;
 const server=createServer({cert:readFileSync(process.env.IAM_FIXTURE_CERT),key:readFileSync(process.env.IAM_FIXTURE_KEY)},async(req,res)=>{
  try{
   const u=new URL(req.url,origin);requests.push({method:req.method,path:u.pathname});
   res.setHeader('Cache-Control','private, no-store');res.setHeader('Referrer-Policy','no-referrer');res.setHeader('X-Content-Type-Options','nosniff');
   if(!['GET','HEAD'].includes(req.method)||req.headers.host!==new URL(origin).host||req.headers.authorization){res.writeHead(403);return res.end()}
   if(u.pathname==='/style.css'){res.setHeader('Content-Type','text/css');return res.end(css)}
   if(u.pathname==='/guard.js'){res.setHeader('Content-Type','text/javascript');return res.end('('+runtimeLogBrowserGuard.toString()+')()')}
   if(u.pathname==='/design/brand.png'){res.setHeader('Content-Type','image/png');return res.end(readFileSync(new URL('../../apps/web/public/design/brand.png',import.meta.url)))}
   if(u.pathname==='/favicon.ico'){res.writeHead(204);return res.end()}
   // Explicit synthetic sign-in exists only in this test host, never in Next/routes.
   if(u.pathname==='/_login'){
    const ws=u.searchParams.get('space')??'A',person=u.searchParams.get('person')??'one',f=fixtures[ws];if(!f||!['one','two'].includes(person)){res.writeHead(400);return res.end()}
    const start=await f.iam.start(),session=await f.iam.callback(f.callback(start,person),start.key);sessions.set(session.key,{f,ws});
    res.setHeader('Set-Cookie',`__Host-dpp-session=${session.key}; Path=/; Secure; HttpOnly; SameSite=Lax`);res.writeHead(303,{Location:'/settings/logs'});return res.end();
   }
   if(u.pathname!=='/settings/logs'){res.writeHead(404);return res.end()}
   const key=req.headers.cookie?.split(';').map(s=>s.trim()).find(s=>s.startsWith('__Host-dpp-session='))?.split('=')[1],session=sessions.get(key);
   const confirm=async()=>{if(!session)throw new LogReadError('AUTHENTICATION_REQUIRED');try{return (await session.f.iam.current(key)).principal}catch{throw new LogReadError('AUTHENTICATION_REQUIRED')}};
   const reader=source.reader(confirm,session?.ws??'A'),query={};for(const [k,v]of u.searchParams){if(k in query)query[k]=[query[k],v].flat();else query[k]=v}
   const view=await loadRuntimeLogs(query,reader);if(view.redirect){res.writeHead(303,{Location:view.redirect});return res.end()}
   res.setHeader('Content-Type','text/html; charset=utf-8');res.setHeader('Content-Security-Policy',"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'");
   const content=React.createElement(ManagementShellView,{path:'/settings/logs',identityEnabled:true,icons},React.createElement('div',{className:'workspace-scope-bar'},'当前工作空间 · '+(session?.ws==='B'?'合成空间乙':'合成空间甲')),React.createElement(RuntimeLogs,{view}));
   res.end('<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/style.css"><title>运行日志</title></head><body>'+renderToStaticMarkup(content)+'<script src="/guard.js"></script></body></html>');
  }catch{errors.push('host failed');res.writeHead(500);res.end('Isolated test unavailable')}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));origin='https://127.0.0.1:'+server.address().port;
 return {origin,source,fixtures,requests,errors,async close(){server.closeAllConnections();await new Promise(resolve=>server.close(resolve));for(const fn of cleaners.reverse())await fn()}};
}
