// Reuse the integrated Next prototype; this host cannot reach connected routes.
import http from 'node:http';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {resolve} from 'node:path';
const root=fileURLToPath(new URL('../../',import.meta.url)),port=4445;
if(process.env.GOVERNANCE_SETTINGS_INTERACTION_DEMO!=='true'||process.env.BROWSER_IAM_MODE&&process.env.BROWSER_IAM_MODE!=='off'||process.env.SETTINGS_ADMIN_UI_ENABLED==='true')throw Error('isolated local interaction preview required');
const require=createRequire(resolve(root,'apps/web/package.json')),next=require('next');
const settingsRoutes=require(resolve(root,'apps/web/lib/settings-interaction-routes.json'));
const app=next({dev:false,dir:resolve(root,'apps/web'),hostname:'127.0.0.1',port});await app.prepare();
const handle=app.getRequestHandler();
http.createServer((request,response)=>{
 const path=(request.url??'').split('?')[0];
 if(request.headers.host!==`127.0.0.1:${port}`||!['GET','HEAD'].includes(request.method??'')||['/api','/auth','/core','/agent'].some(p=>path===p||path.startsWith(p+'/'))){response.writeHead(403,{'Cache-Control':'no-store','Content-Type':'text/plain; charset=utf-8'});return response.end('本入口仅展示本地交互，不提供后端服务。')}
 // Keep public redirects relative. Next's middleware normalizes loopback
 // redirect origins to localhost, which this isolated listener rejects.
 if(Object.hasOwn(settingsRoutes,path)){response.writeHead(307,{'Location':'/#'+settingsRoutes[path],'Cache-Control':'no-store'});return response.end()}
 delete request.headers.authorization;delete request.headers.cookie;delete request.headers.forwarded;
 request.headers['x-forwarded-proto']='http';request.headers['x-forwarded-host']=`127.0.0.1:${port}`;
 // Select the empty demo page before Next middleware. Its rewrite uses an
 // internal loopback request whose Host differs from this public listener.
 // Never render app/page.tsx: that page owns connected business loaders.
 if(path==='/')request.url='/demo'+(request.url?.includes('?')?request.url.slice(request.url.indexOf('?')):'');
 handle(request,response);
}).listen(port,'127.0.0.1',()=>console.log('Settings interaction preview ready at http://127.0.0.1:4445/#settings; local session data only'));
