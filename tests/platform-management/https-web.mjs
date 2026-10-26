// Reuses the existing acceptance TLS host pattern with an independent origin/profile.
import https from 'node:https';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {resolve} from 'node:path';
const root=fileURLToPath(new URL('../../',import.meta.url)),require=createRequire(resolve(root,'apps/web/package.json')),next=require('next');
const port=process.env.MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK==='ISOLATED_DIRECTORY_PRODUCT_READONLY'?4380:process.env.MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK==='ISOLATED_MAIN_SERVICE_COMPAT_READONLY'?4370:process.env.MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK==='ISOLATED_MAIN_SERVICE_READONLY'?4360:process.env.MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK==='ISOLATED_RUNTIME_LOG_READONLY'?4350:4300;
if(process.env.BROWSER_IAM_ORIGIN!==`https://localhost:${port}`||process.env.BROWSER_IAM_PROFILE!=='platform-management')throw Error('independent management origin required');
const dist=process.env.NEXT_DIST_DIR;if(!/^\.next-acceptance-identity-management-[a-z0-9-]+$/.test(dist??''))throw Error('independent build required');
const app=next({dev:false,dir:resolve(root,'apps/web'),hostname:'localhost',port});await app.prepare();const handle=app.getRequestHandler();
https.createServer({cert:readFileSync(process.env.MANAGEMENT_WEB_CERT),key:readFileSync(process.env.MANAGEMENT_WEB_KEY),minVersion:'TLSv1.2'},(request,response)=>{
 if(request.headers.host!==`localhost:${port}`||!request.url?.startsWith('/')||request.url.startsWith('//')||request.headers.authorization){response.writeHead(403);return response.end('Management origin required')}
 delete request.headers.forwarded;request.headers['x-forwarded-proto']='https';request.headers['x-forwarded-host']=`localhost:${port}`;request.headers['x-forwarded-port']=String(port);handle(request,response);
}).listen(port,'127.0.0.1',()=>console.log('Independent management HTTPS page ready; writes absent'));
