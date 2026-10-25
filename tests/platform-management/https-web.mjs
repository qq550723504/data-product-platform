// Reuses the existing acceptance TLS host pattern with an independent origin/profile.
import https from 'node:https';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {resolve} from 'node:path';
const root=fileURLToPath(new URL('../../',import.meta.url)),require=createRequire(resolve(root,'apps/web/package.json')),next=require('next');
if(process.env.BROWSER_IAM_ORIGIN!=='https://localhost:4300'||process.env.BROWSER_IAM_PROFILE!=='platform-management')throw Error('independent management origin required');
const dist=process.env.NEXT_DIST_DIR;if(!/^\.next-acceptance-identity-management-[a-z0-9-]+$/.test(dist??''))throw Error('independent build required');
const app=next({dev:false,dir:resolve(root,'apps/web'),hostname:'localhost',port:4300});await app.prepare();const handle=app.getRequestHandler();
https.createServer({cert:readFileSync(process.env.MANAGEMENT_WEB_CERT),key:readFileSync(process.env.MANAGEMENT_WEB_KEY),minVersion:'TLSv1.2'},(request,response)=>{
 if(request.headers.host!=='localhost:4300'||!request.url?.startsWith('/')||request.url.startsWith('//')||request.headers.authorization){response.writeHead(403);return response.end('Management origin required')}
 delete request.headers.forwarded;request.headers['x-forwarded-proto']='https';request.headers['x-forwarded-host']='localhost:4300';request.headers['x-forwarded-port']='4300';handle(request,response);
}).listen(4300,'127.0.0.1',()=>console.log('Independent management HTTPS page ready; writes absent'));
