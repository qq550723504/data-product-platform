// Synthetic OIDC and native directory contract only. Never use as an identity service.
import https from 'node:https';
import {readFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {randomBytes,createHash} from 'node:crypto';
import {pathToFileURL} from 'node:url';
import {runtimeSource} from './runtime-source.mjs';
const require=createRequire(new URL('../../apps/web/package.json',import.meta.url));
const {generateKeyPair,exportJWK,SignJWT}=await import(pathToFileURL(require.resolve('jose')).href);
if(process.env.MANAGEMENT_FIXTURE_ACK!=='ISOLATED_READONLY_MANAGEMENT')throw Error('isolated fixture required');
const p=JSON.parse(readFileSync(process.env.MANAGEMENT_FIXTURE_PROFILE,'utf8'));
const runtime=p.runtimeLogs===true&&process.env.MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK==='ISOLATED_RUNTIME_LOG_READONLY';
const mainService=p.mainService===true&&process.env.MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK==='ISOLATED_MAIN_SERVICE_READONLY';
const compat=p.mainServiceCompat===true&&process.env.MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK==='ISOLATED_MAIN_SERVICE_COMPAT_READONLY';
if(p.mainService===true&&!mainService||p.mainServiceCompat===true&&!compat||[runtime,mainService,compat].filter(Boolean).length>1)throw Error('disjoint independent main service fixture required');
const idpPort=compat?19170:mainService?19160:runtime?19150:19130;
const logs=runtimeSource(p);
if(p.issuer!==`https://localhost:${idpPort}`||p.origin!==`https://localhost:${compat?4370:mainService?4360:runtime?4350:4300}`)throw Error('exact independent origin required');
const {publicKey,privateKey}=await generateKeyPair('RS256'),jwk=await exportJWK(publicKey);jwk.kid='management-synthetic';
const codes=new Map(),logins=new Map(),tokens=new Map(),held=new Set(),heldUsers=new Set(),heldOrgs=new Set();const state={permission:true,foreign:false,fail:false,holdList:false,waiting:0,reads:0,userPermission:true,userForeign:false,userFail:false,userUnknown:false,holdUser:false,userWaiting:0,userReads:0,removeUser:false,orgPermission:true,orgForeign:false,orgFail:false,orgUnknown:false,orgMissing:false,orgSparse:false,orgExtra:false,holdOrg:false,orgWaiting:0,orgReads:0};
const opaque=()=>randomBytes(32).toString('base64url');
const json=(res,value,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(value))};
async function body(req){let raw='';for await(const chunk of req){raw+=chunk;if(raw.length>16000)throw Error('oversized fixture input')}return raw;}
// OAuth client_secret_basic form-encodes each credential before Base64.
const basic=(req,id,secret)=>{try{const raw=req.headers.authorization??'';if(!raw.startsWith('Basic '))return false;const parts=Buffer.from(raw.slice(6),'base64').toString().split(':');return parts.length===2&&decodeURIComponent(parts[0])===id&&decodeURIComponent(parts[1])===secret}catch{return false}};
const labels={'admin-a':'查询人员 A','admin-b':'查询人员 B',both:'双空间查询人员',ordinary:'无管理权限人员'};
function rows(org){const a=org==='organization-a';return Array.from({length:a?58:17},(_,i)=>({id:`grant-${a?'a':'b'}-${String(i).padStart(3,'0')}`,creationDate:'2026-10-24T00:00:00Z',changeDate:'2026-10-24T00:00:00Z',project:{id:'governance'},organization:{id:org},user:{id:`person-${a?'a':'b'}-${i}`,displayName:`${a?'空间甲':'空间乙'}成员 ${String(i).padStart(3,'0')}${i===57?' 搜索目标':''}`,preferredLoginName:`person-${a?'a':'b'}-${i}@fixture.invalid`},state:i%7===0?'STATE_INACTIVE':'STATE_ACTIVE',roles:[{key:i%3===0?'audit_viewer':'management_viewer'}]}));}
const server=https.createServer({cert:readFileSync(p.cert),key:readFileSync(p.key),minVersion:'TLSv1.2'},async(req,res)=>{
 try{
  const u=new URL(req.url,p.issuer);
  if((runtime||compat)&&await logs.handle(req,res,u))return;
  if(u.pathname==='/authorize'&&req.method==='GET'){
   if(u.searchParams.get('client_id')!=='dpp-management-rp'||u.searchParams.get('redirect_uri')!==p.origin+'/auth/callback'||u.searchParams.get('code_challenge_method')!=='S256'||u.searchParams.get('scope')!=='openid platform-management')return json(res,{},400);
   const id=opaque();logins.set(id,u.searchParams);res.writeHead(200,{'Content-Type':'text/html; charset=utf-8','Cache-Control':'no-store'});return res.end(`<!doctype html><html lang="zh-CN"><meta name="viewport" content="width=device-width,initial-scale=1"><h1>隔离身份验收</h1><p>仅合成身份，不连接企业人员目录。</p><form method="post" action="/login"><input type="hidden" name="request" value="${id}">${Object.entries(labels).map(([key,label])=>`<p><button name="person" value="${key}">${label}</button></p>`).join('')}</form></html>`);
  }
  if(u.pathname==='/login'&&req.method==='POST'){
   const b=new URLSearchParams(await body(req)),pending=logins.get(b.get('request'));logins.delete(b.get('request'));const subject=b.get('person');if(!pending||!labels[subject])return json(res,{},400);
   const code=opaque();codes.set(code,{subject,nonce:pending.get('nonce'),challenge:pending.get('code_challenge')});const target=new URL(p.origin+'/auth/callback');target.searchParams.set('code',code);target.searchParams.set('state',pending.get('state'));res.writeHead(303,{Location:target.href,'Cache-Control':'no-store'});return res.end();
  }
  if(u.pathname==='/token'&&req.method==='POST'){
   if(!basic(req,'dpp-management-rp',p.rpSecret))return json(res,{},401);const b=new URLSearchParams(await body(req)),code=codes.get(b.get('code'));codes.delete(b.get('code'));
   if(!code||b.get('redirect_uri')!==p.origin+'/auth/callback'||createHash('sha256').update(b.get('code_verifier')??'').digest('base64url')!==code.challenge)return json(res,{},400);
   const now=Math.floor(Date.now()/1000),access=opaque();tokens.set(access,{subject:code.subject,expires:now+600});const token=await new SignJWT({iss:p.issuer,sub:code.subject,aud:'dpp-management-rp',iat:now,exp:now+600,nonce:code.nonce}).setProtectedHeader({alg:'RS256',kid:jwk.kid}).sign(privateKey);
   return json(res,{access_token:access,id_token:token,expires_in:600,token_type:'Bearer'});
  }
  if(u.pathname==='/jwks'&&req.method==='GET')return json(res,{keys:[jwk]});
  if(u.pathname==='/introspect'&&req.method==='POST'){
   if(!basic(req,'dpp-management-api',p.resourceSecret))return json(res,{},401);const b=new URLSearchParams(await body(req)),t=tokens.get(b.get('token'));if(!t||t.revoked||t.expires<=Date.now()/1000)return json(res,{active:false});return json(res,{active:true,iss:p.issuer,sub:t.subject,aud:['dpp-management-api'],client_id:'dpp-management-rp',scope:'openid platform-management',exp:t.expires,iat:t.expires-600,token_type:'Bearer'});
  }
  if(u.pathname==='/auth/v1/permissions/zitadel/me/_search'&&req.method==='POST'){
   if(req.headers.authorization!=='Bearer '+p.readCredential||!['organization-a','organization-b'].includes(req.headers['x-zitadel-orgid']))return json(res,{},403);return json(res,{result:[...(state.permission?['user.grant.read']:[]),...(state.userPermission?['user.read']:[]),...(state.orgPermission?['org.read']:[])]});
  }
  if(u.pathname==='/v2/organizations/_search'&&req.method==='POST'){
   const org=req.headers['x-zitadel-orgid'];if(req.headers.authorization!=='Bearer '+p.readCredential||!['organization-a','organization-b'].includes(org))return json(res,{},403);
   const q=JSON.parse(await body(req)),expected={query:{offset:'0',limit:1,asc:true},sortingColumn:'ORGANIZATION_FIELD_NAME_NAME',queries:[{idQuery:{id:org}}]};if(JSON.stringify(q)!==JSON.stringify(expected))return json(res,{},400);
   state.orgReads++;if(state.orgFail)return json(res,{},503);
   if(state.holdOrg){state.orgWaiting++;const released=await new Promise(resolve=>{const release=()=>{clearTimeout(timer);heldOrgs.delete(release);resolve(true)},timer=setTimeout(()=>{heldOrgs.delete(release);resolve(false)},5000);heldOrgs.add(release)});state.orgWaiting--;if(!released)return json(res,{},503)}
   if(state.orgMissing)return json(res,{details:{totalResult:'0'},result:[]});
   const record={id:state.orgForeign?'organization-foreign':org,state:state.orgUnknown?'ORGANIZATION_STATE_UNSPECIFIED':org==='organization-a'?'ORGANIZATION_STATE_ACTIVE':'ORGANIZATION_STATE_INACTIVE',...(state.orgSparse?{}:{name:org==='organization-a'?'甲方目录组织':'乙方目录组织',details:{creationDate:'2026-10-24T00:00:00Z',changeDate:'2026-10-25T00:00:00Z',resourceOwner:'synthetic-identity-instance'}}),primaryDomain:'PRIVATE_ORG_DOMAIN_NEVER_RENDER',metadata:{private:'PRIVATE_ORG_METADATA_NEVER_RENDER'}};
   return json(res,{details:{totalResult:state.orgExtra?'2':'1'},result:state.orgExtra?[record,{...record,id:'organization-other'}]:[record]});
  }
  if(u.pathname.startsWith('/v2/users/')&&req.method==='GET'){
   const org=req.headers['x-zitadel-orgid'];if(req.headers.authorization!=='Bearer '+p.readCredential||!['organization-a','organization-b'].includes(org))return json(res,{},403);
   state.userReads++;if(state.userFail)return json(res,{},503);const id=decodeURIComponent(u.pathname.slice('/v2/users/'.length)),row=rows(org).find(r=>r.user.id===id);if(!row)return json(res,{},404);
   if(state.holdUser){state.userWaiting++;const released=await new Promise(resolve=>{const release=()=>{clearTimeout(timer);heldUsers.delete(release);resolve(true)},timer=setTimeout(()=>{heldUsers.delete(release);resolve(false)},5000);heldUsers.add(release)});state.userWaiting--;if(!released)return json(res,{},503)}
   return json(res,{user:{userId:id,username:row.user.preferredLoginName,state:state.userUnknown?'USER_STATE_UNKNOWN':Number(id.split('-').at(-1))%5===0?'USER_STATE_INACTIVE':'USER_STATE_ACTIVE',details:{resourceOwner:state.userForeign?'organization-foreign':org,creationDate:row.creationDate,changeDate:row.changeDate},human:{profile:{displayName:row.user.displayName},email:{email:'PRIVATE_USER_EMAIL_NEVER_RENDER'},phone:{phone:'PRIVATE_USER_PHONE_NEVER_RENDER'}}}});
  }
  if(u.pathname==='/zitadel.authorization.v2.AuthorizationService/ListAuthorizations'&&req.method==='POST'){
   if(req.headers.authorization!=='Bearer '+p.readCredential)return json(res,{},403);state.reads++;if(state.fail)return json(res,{},503);const b=JSON.parse(await body(req)),filters=b.filters??[],org=filters.find(f=>f.organizationId)?.organizationId?.id,project=filters.find(f=>f.projectId)?.projectId?.id;
   if(!['organization-a','organization-b'].includes(org)||project!=='governance')return json(res,{},400);
   const role=filters.find(f=>f.roleKey)?.roleKey?.key,status=filters.find(f=>f.state)?.state?.state,ids=filters.find(f=>f.authorizationIds)?.authorizationIds?.ids;
   const matched=rows(org).filter(r=>(!state.removeUser||r.user.id!=='person-a-2')&&(!role||r.roles.some(v=>v.key===role))&&(!status||r.state===status)&&(!ids||ids.includes(r.id))),items=matched.slice(b.pagination.offset,b.pagination.offset+b.pagination.limit);
   if(state.holdList){state.waiting++;const released=await new Promise(resolve=>{const release=()=>{clearTimeout(timer);held.delete(release);resolve(true)},timer=setTimeout(()=>{held.delete(release);resolve(false)},5000);held.add(release)});state.waiting--;if(!released)return json(res,{},503)}
   if(state.foreign)for(const r of items)r.organization.id='organization-foreign';return json(res,{pagination:{totalResult:String(matched.length)},authorizations:items});
  }
  if(u.pathname==='/logout'){const target=new URL(p.origin+'/auth/logged-out');res.writeHead(303,{Location:target.href});return res.end();}
  if(u.pathname==='/_control'&&req.headers['x-fixture-control']===p.control){
   let testAccessToken;
   if(req.method==='POST'){
    const b=JSON.parse(await body(req));if(runtime||compat)logs.control(b);for(const k of ['permission','foreign','fail','holdList','userPermission','userForeign','userFail','userUnknown','holdUser','removeUser','orgPermission','orgForeign','orgFail','orgUnknown','orgMissing','orgSparse','orgExtra','holdOrg'])if(typeof b[k]==='boolean')state[k]=b[k];
    if(Object.hasOwn(labels,b.revokeSubject))for(const t of tokens.values())if(t.subject===b.revokeSubject)t.revoked=true;
    if(Object.hasOwn(labels,b.expireSubject))for(const t of tokens.values())if(t.subject===b.expireSubject)t.expires=Math.floor(Date.now()/1000)-1;
    if(Object.hasOwn(labels,b.issueSubject)){testAccessToken=opaque();tokens.set(testAccessToken,{subject:b.issueSubject,expires:Math.floor(Date.now()/1000)+600})}
    if(!state.holdList)for(const release of held)release();
    if(!state.holdUser)for(const release of heldUsers)release();
    if(!state.holdOrg)for(const release of heldOrgs)release();
   }
   return json(res,{...state,...(runtime||compat)?logs.state:{},...(testAccessToken?{testAccessToken}:{})});
  }
  if(u.pathname==='/healthz')return json(res,{synthetic:true,businessWrites:false,realSources:'UNCONFIGURED'});
  return json(res,{},404);
 }catch{return json(res,{error:'synthetic fixture request failed'},400)}
});
server.listen(idpPort,'127.0.0.1',()=>console.log('Synthetic identity and directory contract ready; no real accounts or providers'));
