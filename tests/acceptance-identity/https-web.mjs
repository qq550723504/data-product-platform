// Local acceptance TLS front door. It forwards only to the owned loopback
// Next server, overwrites forwarded identity-independent transport headers,
// and never logs cookies, authorization codes or private configuration.
import https from 'node:https';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {dirname,resolve} from 'node:path';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../..');
const require=createRequire(resolve(root,'apps/web/package.json'));
const next=require('next');
const origin=new URL(process.env.BROWSER_IAM_ORIGIN??'');
if(origin.origin!=='https://localhost:4290'||process.env.BROWSER_IAM_PROFILE!=='business-acceptance')throw new Error('Explicit isolated identity origin required');
// Bind Next's canonical hostname/port to the actual TLS listener. A proxy with
// a different internal port changes NextRequest.url and correctly fails CSRF.
const dist=process.env.ACCEPTANCE_IDENTITY_WEB_DIST??'.next-acceptance-identity-1a';
if(!/^\.next-acceptance-identity-(1a|1b(?:-[a-z0-9-]{1,40})?)$/.test(dist))throw new Error('Explicit independent build required');
process.env.NEXT_DIST_DIR=dist;
const app=next({dev:false,dir:resolve(root,'apps/web'),hostname:'localhost',port:4290});
await app.prepare();const handle=app.getRequestHandler();
const server=https.createServer({cert:readFileSync(process.env.ACCEPTANCE_WEB_CERT),key:readFileSync(process.env.ACCEPTANCE_WEB_KEY),minVersion:'TLSv1.2'},(request,response)=>{
 if(request.headers.host!==origin.host||!request.url?.startsWith('/')||request.url.startsWith('//')||request.headers.authorization){response.writeHead(403);response.end('Isolated identity origin required');return;}
 delete request.headers.forwarded;
 request.headers['x-forwarded-proto']='https';request.headers['x-forwarded-host']=origin.host;request.headers['x-forwarded-port']='4290';
 handle(request,response);
});
server.listen(4290,'127.0.0.1',()=>console.log('Isolated identity HTTPS front door ready; business writes disabled'));
