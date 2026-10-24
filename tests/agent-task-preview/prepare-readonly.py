"""Explicit one-time preparation of a new DB in the existing owned container.
Never resets/reuses a DB. Secrets stay in the ignored local runtime profile.
"""
import json, os, pathlib, secrets, subprocess, urllib.parse
ROOT=pathlib.Path(__file__).resolve().parents[2]
CONTAINER='dpp-agent-q2-test-postgres-20261023'
DATABASE='agent_q2_acceptance_20261024'
ROLE='agent_q2_reader_20261024'
RUN=ROOT/'.data/agent-q2-preview'
def sql(database,query,user='postgres'):
 r=subprocess.run(['docker','exec','-i',CONTAINER,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U',user,'-d',database],input=query,text=True,capture_output=True)
 if r.returncode: raise RuntimeError('database preparation/query failed (details withheld to protect credentials)')
 return r.stdout.strip()
def fingerprint(database):
 names=sql(database,"BEGIN READ ONLY; SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename; ROLLBACK;").splitlines()
 queries=["SELECT '"+n.replace("'","''")+"' AS name,count(*) AS count,md5(coalesce(string_agg(to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text),'')) AS hash FROM public.\""+n.replace('"','""')+"\" t" for n in names]
 return sorted(json.loads(sql(database,'BEGIN READ ONLY; SELECT json_agg(x) FROM ('+' UNION ALL '.join(queries)+') x; ROLLBACK;')),key=lambda row:row['name'])
def main():
 meta=json.loads(subprocess.check_output(['docker','inspect',CONTAINER],text=True))[0]
 if not meta['State']['Running'] or meta['Config']['Labels'].get('codex.scope')!='agent-q2-test' or meta['HostConfig']['PortBindings'].get('5432/tcp')!=[{'HostIp':'127.0.0.1','HostPort':'25494'}]:raise RuntimeError('owned running container required')
 if sql('postgres',"SELECT count(*) FROM pg_database WHERE datname='"+DATABASE+"'")!='0':raise RuntimeError('database already exists; do not rerun preparation')
 if sql('postgres',"SELECT count(*) FROM pg_roles WHERE rolname='"+ROLE+"'")!='0':raise RuntimeError('role already exists; do not replace')
 RUN.mkdir(parents=True,exist_ok=True)
 (RUN/'original-before.json').write_text(json.dumps(fingerprint('agent_q2_test')),encoding='utf-8')
 sql('postgres','CREATE DATABASE '+DATABASE)
 env=dict(os.environ);settings=dict(x.split('=',1) for x in meta['Config']['Env'] if '=' in x)
 password=settings['POSTGRES_PASSWORD']
 env['READONLY_PREPARE_DSN']='postgres://postgres:'+urllib.parse.quote(password,safe='')+'@127.0.0.1:25494/'+DATABASE+'?sslmode=disable'
 r=subprocess.run([str(RUN/'readonly-prepare.exe')],cwd=ROOT,env=env,capture_output=True,text=True)
 if r.returncode:raise RuntimeError('fixture preparation failed (details withheld to protect credentials)')
 reader_password=secrets.token_urlsafe(32)
 tables='dataset dataset_version data_resource quality_result quality_finding quality_assessment_attempt quality_assessment_attempt_outcome evidence evidence_relation evidence_snapshot evidence_snapshot_item audit_event cost_event cost_allocation data_product product_version product_release product_release_dataset dataset_version_lineage execution workflow workflow_version execution_dependency_preparation execution_dependency_binding execution_mapping_usage entity entity_type entity_match_job entity_mapping entity_mapping_decision agent_task agent_plan_revision agent_plan_input agent_plan_action agent_approval agent_action_request agent_action_disposition agent_command_receipt dataset_certification certification_disposition certification_profile delivery_authorization_fence'.split()
 profiles='purpose action consumer delivery quality_dimension critical_rule rights_purpose rights_action rights_consumer rights_scope'.split()
 tables+=['certification_profile_'+x for x in profiles]
 sql(DATABASE,"BEGIN; CREATE ROLE "+ROLE+" LOGIN PASSWORD '"+reader_password+"'; ALTER ROLE "+ROLE+" SET default_transaction_read_only=on; REVOKE ALL ON DATABASE "+DATABASE+" FROM PUBLIC; REVOKE CREATE ON SCHEMA public FROM PUBLIC; REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC; GRANT CONNECT ON DATABASE "+DATABASE+" TO "+ROLE+"; GRANT USAGE ON SCHEMA public TO "+ROLE+"; GRANT SELECT ON "+','.join(tables)+" TO "+ROLE+"; COMMIT;")
 dsn='postgres://'+ROLE+':'+urllib.parse.quote(reader_password,safe='')+'@127.0.0.1:25494/'+DATABASE+'?sslmode=disable'
 profile=RUN/'readonly-profile.json'
 profile.touch(exist_ok=False)
 if os.name=='nt':
  owner=subprocess.check_output(['whoami'],text=True).strip()
  subprocess.run(['icacls',str(profile),'/inheritance:r','/grant:r',owner+':(F)','SYSTEM:(F)'],check=True,capture_output=True)
 profile.write_text(json.dumps({'database':DATABASE,'dsn':dsn,'role':ROLE}),encoding='utf-8')
 (RUN/'readonly-before.json').write_text(json.dumps(fingerprint(DATABASE)),encoding='utf-8')
 print('Independent database and SELECT-only role prepared; original database untouched.')
if __name__=='__main__':main()
