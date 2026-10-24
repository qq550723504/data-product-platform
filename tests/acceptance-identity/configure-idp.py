"""Configure the owned synthetic realm with standard Keycloak subject mapping."""
import json,ssl,urllib.request,urllib.parse,os
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];RUN=ROOT/'.data/acceptance-identity-1a'
def main():
 if os.environ.get('ACCEPTANCE_IDENTITY_ACK')!='ISOLATED_READONLY_1A':raise RuntimeError('explicit isolated configuration required')
 p=json.loads((RUN/'profile.json').read_text(encoding='utf-8'))
 if p['issuer']!='https://localhost:19120/realms/dpp-acceptance-1a':raise RuntimeError('exact fixture required')
 ctx=ssl.create_default_context(cafile=str(RUN/'ca.crt'))
 data=urllib.parse.urlencode({'grant_type':'password','client_id':'admin-cli','username':'fixture-admin','password':p['adminPassword']}).encode()
 with urllib.request.urlopen(urllib.request.Request('https://localhost:19120/realms/master/protocol/openid-connect/token',data=data),context=ctx,timeout=10) as r:token=json.load(r)['access_token']
 def call(path,body=None,method='GET'):
  req=urllib.request.Request('https://localhost:19120/admin/realms/dpp-acceptance-1a/'+path,data=None if body is None else json.dumps(body).encode(),headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'},method=method)
  with urllib.request.urlopen(req,context=ctx,timeout=10) as r:
   raw=r.read();return json.loads(raw) if raw else None
 scope=next(v for v in call('client-scopes') if v['name']=='business-acceptance')
 mappers=call('client-scopes/'+scope['id']+'/protocol-mappers/models')
 if not any(v['protocolMapper']=='oidc-sub-mapper' for v in mappers):
  call('client-scopes/'+scope['id']+'/protocol-mappers/models',{'name':'subject','protocol':'openid-connect','protocolMapper':'oidc-sub-mapper','config':{'access.token.claim':'true','introspection.token.claim':'true'}},'POST')
 print('Synthetic realm standard subject mapper configured; no token or credential exported.')
if __name__=='__main__':main()
