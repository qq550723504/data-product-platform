// Real HTTP/OIDC/PG negative cases; controls belong only to the new synthetic host.
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
const require=createRequire(new URL('../browser/package.json',import.meta.url)),{request,expect}=require('@playwright/test'),{Pool}=createRequire(new URL('../../apps/web/package.json',import.meta.url))('pg');
if(process.env.MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK!=='ISOLATED_MAIN_SERVICE_COMPAT_READONLY')throw Error('new isolated compatibility fixture required');
const fixture=new URL('../../.data/management-main-service-compat-20261026/',import.meta.url),p=JSON.parse(readFileSync(new URL('profile.json',fixture),'utf8'));
assert.equal(p.mainServiceCompat,true);assert.equal(p.issuer,'https://localhost:19170');assert.equal(p.origin,'https://localhost:4370');assert.equal(p.database,'management_main_service_compat_20261026');
const u=new URL(p.adminDSN);assert.equal(u.hostname,'127.0.0.1');assert.equal(u.port,'25570');assert.equal(u.pathname,'/'+p.database);
const output=process.env.MANAGEMENT_COMPAT_BOUNDARY_OUTPUT??'management-main-service-compat-boundaries-20261026';assert.match(output,/^management-main-service-compat-boundaries-[a-z0-9-]+$/);const out=new URL('../../.data/'+output+'/',import.meta.url);if(existsSync(out))throw Error('prior boundary evidence retained');mkdirSync(out);
const db=new Pool({connectionString:p.adminDSN,ssl:{ca:readFileSync(p.cert),rejectUnauthorized:true}}),http=await request.newContext({ignoreHTTPSErrors:true}),results=[],ws='11111111-1111-4111-8111-111111111111',actor='83000000-0000-4000-8000-000000000001';
const cases=[['members','holdList','waiting','permission'],['users?userId=person-a-2','holdUser','userWaiting','userPermission'],['organization','holdOrg','orgWaiting','orgPermission']].flatMap(([path,hold,waiting,permission])=>['token-revoked','token-expired','grant-revoked'].map(mode=>({path,hold,waiting,permission,mode,name:path.split('?')[0]+'/'+mode+'/source-permission-failed'})));
async function control(data={}){const r=await http.post(p.issuer+'/_control',{headers:{'x-fixture-control':p.control},data,maxRedirects:0});assert.equal(r.status(),200);return r.json()}
async function facts(){const all={};for(const table of ['audit_event','execution','agent_task','agent_action_request','delivery_provider_attempt','outbox_event'])all[table]=(await db.query("SELECT count(*)::text AS count,md5(COALESCE(string_agg(row_to_json(t)::text,'|' ORDER BY id),'')) AS hash FROM "+table+' t')).rows[0];return all}
let active='setup',before;
try{
 before=await facts();
 for(const test of cases){active=test.name;let pending,restore;
  try{
   restore=(await db.query('SELECT capabilities,valid_until,enabled FROM platform_workspace_grant WHERE actor_id=$1 AND workspace_id=$2',[actor,ws])).rows[0];assert.ok(restore);
   const token=(await control({issueSubject:'admin-a',[test.hold]:true})).testAccessToken;
   pending=http.get('https://localhost:18570/api/v1/workspaces/'+ws+'/management/'+test.path,{headers:{Authorization:'Bearer '+token},maxRedirects:0});
   await expect.poll(async()=>(await control())[test.waiting],{timeout:3000}).toBe(1);
   if(test.mode==='grant-revoked')await db.query("UPDATE platform_workspace_grant SET capabilities=ARRAY['BUSINESS_ACCEPTANCE_READ'],revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2",[actor,ws]);
   await control({[test.permission]:false,[test.hold]:false,...(test.mode==='token-revoked'?{revokeSubject:'admin-a'}:test.mode==='token-expired'?{expireSubject:'admin-a'}:{})});
   const response=await pending,body=await response.json();assert.equal(response.status(),test.mode==='grant-revoked'?403:401,'post-I/O authority must take precedence over source failure');assert.equal(Object.hasOwn(body,'data'),false);assert.equal(body.code,test.mode==='grant-revoked'?'MANAGEMENT_ACCESS_DENIED':'AUTHENTICATION_REQUIRED');results.push({name:test.name,status:'PASS'});console.log('PASS '+test.name);
  }finally{
   await control({holdList:false,holdUser:false,holdOrg:false,permission:true,userPermission:true,orgPermission:true});await pending?.catch(()=>{});
   if(restore)await db.query('UPDATE platform_workspace_grant SET capabilities=$3,valid_until=$4,enabled=$5,revision=revision+1 WHERE actor_id=$1 AND workspace_id=$2',[actor,ws,restore.capabilities,restore.valid_until,restore.enabled]);
  }
 }
 active='unchanged-business-facts';assert.deepEqual(await facts(),before);results.push({name:active,status:'PASS'});
}catch(e){let error=e.message.replace(/Bearer\s+[^\s]+/gi,'Bearer [redacted]');for(const value of Object.values(p))if(typeof value==='string'&&value.length>8)error=error.split(value).join('[redacted]');results.push({name:active,status:'FAIL',error});console.error('FAIL '+active+': '+error);process.exitCode=1}
finally{for(const test of cases)if(!results.some(r=>r.name===test.name))results.push({name:test.name,status:'NOT_RUN'});writeFileSync(new URL('boundary-results.json',out),JSON.stringify({transport:'Playwright APIRequestContext',results,realSources:'UNCONFIGURED',businessWrites:false,models:false,dispatch:false},null,2));await http.dispose();await db.end()}
