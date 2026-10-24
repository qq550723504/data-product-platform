"""Minimal roles/config for the exact owned 1A database; never an original DSN."""
import json,os,subprocess,urllib.parse
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];RUN=ROOT/'.data/acceptance-identity-1a'
TABLES=['platform_actor_state','platform_subject_binding','platform_workspace_grant','dataset','dataset_version','delivery_operation','data_contract','contract_version','acceptance_party_representation','acceptance_responsibility_binding','acceptance_standard_applicability','acceptance_binding_disposition']
def main():
 if os.environ.get('ACCEPTANCE_IDENTITY_ACK')!='ISOLATED_READONLY_1A':raise RuntimeError('explicit isolated configuration required')
 p=json.loads((RUN/'profile.json').read_text(encoding='utf-8'));resources=json.loads((RUN/'resources.json').read_text(encoding='utf-8'))
 if p['database']!='acceptance_1a_20261024' or p['postgres']!='dpp-acceptance-1a-postgres-20261024':raise RuntimeError('exact independent fixture required')
 inspection=subprocess.run(['docker','inspect',p['postgres']],capture_output=True,encoding='utf-8',check=True)
 if json.loads(inspection.stdout)[0]['Id']!=resources['postgres']['id']:raise RuntimeError('container owner changed')
 for key in ['apiPassword','rpPassword']:
  if not p[key] or any(v not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-' for v in p[key]):raise RuntimeError('invalid generated secret')
 sql="REVOKE ALL ON DATABASE acceptance_1a_20261024 FROM PUBLIC;REVOKE CREATE ON SCHEMA public FROM PUBLIC;REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;"
 for role,key in [('acceptance_1a_api','apiPassword'),('acceptance_1a_rp','rpPassword')]:
  sql+=f"DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='{role}') THEN CREATE ROLE {role} LOGIN;END IF;END $$;ALTER ROLE {role} WITH NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '{p[key]}';GRANT CONNECT ON DATABASE acceptance_1a_20261024 TO {role};GRANT USAGE ON SCHEMA public TO {role};REVOKE ALL ON ALL TABLES IN SCHEMA public FROM {role};"
 sql+='ALTER ROLE acceptance_1a_api SET default_transaction_read_only=on;GRANT SELECT ON '+','.join(TABLES)+' TO acceptance_1a_api;GRANT SELECT,INSERT,DELETE ON platform_browser_transaction,platform_browser_session TO acceptance_1a_rp;'
 proc=subprocess.run(['docker','exec','-i',p['postgres'],'psql','-U','postgres','-d',p['database'],'-v','ON_ERROR_STOP=1'],input=sql,capture_output=True,encoding='utf-8')
 if proc.returncode:raise RuntimeError('isolated minimal-role setup failed; output withheld')
 def dsn(role,password):return 'postgres://'+role+':'+urllib.parse.quote(password,safe='')+'@127.0.0.1:25495/'+p['database']
 issuer=p['issuer'];origin=p['origin']
 env={'NODE_ENV':'production','NODE_EXTRA_CA_CERTS':str(RUN/'ca.crt'),'BROWSER_IAM_MODE':'oidc','BROWSER_IAM_PROFILE':'business-acceptance','BROWSER_IAM_ORIGIN':origin,'BROWSER_IAM_ISSUER':issuer,'BROWSER_IAM_CLIENT_ID':'dpp-acceptance-rp','BROWSER_IAM_CLIENT_SECRET':p['rpSecret'],'BROWSER_IAM_AUTHORIZATION_ENDPOINT':issuer+'/protocol/openid-connect/auth','BROWSER_IAM_TOKEN_ENDPOINT':issuer+'/protocol/openid-connect/token','BROWSER_IAM_JWKS_URI':issuer+'/protocol/openid-connect/certs','BROWSER_IAM_LOGOUT_ENDPOINT':issuer+'/protocol/openid-connect/logout','BROWSER_IAM_PRINCIPAL_ENDPOINT':'https://localhost:18498/internal/identity/business-acceptance-principal','BROWSER_IAM_SCOPE':'openid business-acceptance','BROWSER_IAM_ENCRYPTION_KEY':p['encryptionKey'],'BROWSER_IAM_DATABASE_URL':dsn('acceptance_1a_rp',p['rpPassword']),'BROWSER_IAM_DATABASE_CA_FILE':str(RUN/'ca.crt'),'BROWSER_IAM_WORKSPACE_ID':'11111111-1111-4111-8111-111111111111','BROWSER_IAM_WORKSPACES':json.dumps([{'id':'11111111-1111-4111-8111-111111111111','label':'验收工作空间 A'},{'id':'12121212-1212-4212-8212-121212121212','label':'验收工作空间 B'}]),'AGENT_TASKS_UI_WRITE_ENABLED':'false','AGENT_QUALITY_UI_WRITE_ENABLED':'false'}
 (RUN/'web-env.json').write_text(json.dumps(env,ensure_ascii=False),encoding='utf-8')
 api={'ACCEPTANCE_IDENTITY_ACK':'ISOLATED_READONLY_1A','ACCEPTANCE_IDENTITY_DSN':dsn('acceptance_1a_api',p['apiPassword'])+'?sslmode=verify-full&sslrootcert='+urllib.parse.quote(str(RUN/'ca.crt'),safe=''),'ACCEPTANCE_IDENTITY_CA':str(RUN/'ca.crt'),'ACCEPTANCE_IDENTITY_CERT':str(RUN/'server.crt'),'ACCEPTANCE_IDENTITY_KEY':str(RUN/'server.key'),'ACCEPTANCE_IDENTITY_ISSUER':issuer,'ACCEPTANCE_IDENTITY_RESOURCE_CLIENT':'dpp-acceptance-api','ACCEPTANCE_IDENTITY_RESOURCE_SECRET':p['resourceSecret']}
 (RUN/'api-env.json').write_text(json.dumps(api),encoding='utf-8')
 print('Independent API SELECT-only and RP ephemeral-session-only roles/configured. Business writes remain absent.')
if __name__=='__main__':main()
