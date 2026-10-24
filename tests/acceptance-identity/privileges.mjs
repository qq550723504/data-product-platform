import {readFileSync,writeFileSync} from 'node:fs';import {createRequire} from 'node:module';import {fileURLToPath} from 'node:url';import {dirname,resolve} from 'node:path';import assert from 'node:assert/strict';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'../..'),run=resolve(root,'.data/acceptance-identity-1a');
const p=JSON.parse(readFileSync(resolve(run,'profile.json'),'utf8'));assert.equal(p.database,'acceptance_1a_20261024');
const {Pool}=createRequire(resolve(root,'apps/web/package.json'))('pg');
const options={host:'127.0.0.1',port:25495,database:p.database,ssl:{ca:readFileSync(resolve(run,'ca.crt'),'utf8'),rejectUnauthorized:true}};
const admin=new Pool({...options,user:'postgres',password:p.pgPassword});const results=[];
try{for(const [role,password,allowed] of [['acceptance_1a_api',p.apiPassword,['platform_actor_state','platform_subject_binding','platform_workspace_grant','dataset','dataset_version','delivery_operation','data_contract','contract_version','acceptance_party_representation','acceptance_responsibility_binding','acceptance_standard_applicability','acceptance_binding_disposition']],['acceptance_1a_rp',p.rpPassword,['platform_browser_transaction','platform_browser_session']]]){
 const pool=new Pool({...options,user:role,password});try{
  const tables=(await admin.query("SELECT tablename FROM pg_tables WHERE schemaname='public'")).rows.map(v=>v.tablename);
  for(const table of tables)for(const privilege of ['SELECT','INSERT','UPDATE','DELETE','TRUNCATE','REFERENCES','TRIGGER']){
   const actual=(await pool.query('SELECT has_table_privilege(current_user,$1,$2) AS allowed',[table,privilege])).rows[0].allowed;
   const expected=allowed.includes(table)&&(privilege==='SELECT'||role.endsWith('_rp')&&['INSERT','DELETE'].includes(privilege));assert.equal(actual,expected,role+'/'+table+'/'+privilege);
  }
  const limits=(await pool.query("SELECT has_database_privilege(current_user,current_database(),'TEMP') AS temp,has_schema_privilege(current_user,'public','CREATE') AS create_schema")).rows[0];assert.equal(limits.temp,false);assert.equal(limits.create_schema,false);
  const funcs=(await pool.query("SELECT count(*) AS n FROM pg_proc p JOIN pg_namespace n ON p.pronamespace=n.oid WHERE n.nspname='public' AND has_function_privilege(current_user,p.oid,'EXECUTE')")).rows[0];assert.equal(Number(funcs.n),0);
  if(role.endsWith('_api'))assert.equal((await pool.query('SHOW default_transaction_read_only')).rows[0].default_transaction_read_only,'on');
  results.push({role,tables:allowed.length,status:'PASS',businessMutationPrivileges:false});
 }finally{await pool.end()}
}}finally{await admin.end()}
writeFileSync(resolve(run,'privilege-results.json'),JSON.stringify(results,null,2));console.log('PASS exact API SELECT-only and RP session-only privileges; no schema/temp/public-function access');
