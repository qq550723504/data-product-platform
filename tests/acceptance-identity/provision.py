"""One-time isolated Keycloak/TLS PostgreSQL fixture. Never touches retained DBs."""
import json, os, secrets, subprocess, sys, time, urllib.parse
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tests'))
from fixture_preservation import container_metadata,capture_retained,verify_retained
RUN=ROOT/'.data/acceptance-identity-1a'
PG='dpp-acceptance-1a-postgres-20261024'
IDP='dpp-acceptance-1a-keycloak-20261024'
KEYCLOAK='quay.io/keycloak/keycloak@sha256:d79bc4bf1c54e802735ef91926b5c003de1fbb50b1a93382611972277219c9ad'
DATABASE='acceptance_1a_20261024'
ISSUER='https://localhost:19120/realms/dpp-acceptance-1a'
ORIGIN='https://localhost:4290'
PEOPLE=['provider','receiver','confirmer','reviewer','archivist','disposer']
def call(args,**kwargs):
    p=subprocess.run(args,capture_output=True,encoding='utf-8',**kwargs)
    if p.returncode: raise RuntimeError('isolated preparation step failed; command output withheld')
    return p.stdout
def metadata(name):
    return container_metadata(name,include_image=True)
def main():
    if os.environ.get('ACCEPTANCE_IDENTITY_PREPARE_ACK')!='NEW_ISOLATED_1A_ONLY':raise RuntimeError('explicit isolated preparation required')
    if (RUN/'profile.json').exists() or metadata(PG) or metadata(IDP):raise RuntimeError('existing 1A resources found; inspect and reuse, never overwrite')
    os.umask(0o077)
    RUN.mkdir(parents=True,exist_ok=True,mode=0o700)
    before=capture_retained(('original',),RUN/'retained-comparison.json',observer=metadata)
    (RUN/'retained-before.json').write_text(json.dumps(before),encoding='utf-8')
    pw={name:secrets.token_urlsafe(24) for name in PEOPLE}
    profile={'database':DATABASE,'postgres':PG,'keycloak':IDP,'issuer':ISSUER,'origin':ORIGIN,'pgPassword':secrets.token_urlsafe(32),'apiPassword':secrets.token_urlsafe(32),'rpPassword':secrets.token_urlsafe(32),'rpSecret':secrets.token_urlsafe(32),'resourceSecret':secrets.token_urlsafe(32),'adminPassword':secrets.token_urlsafe(32),'encryptionKey':secrets.token_urlsafe(32),'people':{name:{'username':name,'password':pw[name],'subject':f'72000000-0000-4000-8000-{i+1:012d}','actor':f'73000000-0000-4000-8000-{i+1:012d}'} for i,name in enumerate(PEOPLE)}}
    (RUN/'profile.json').write_text(json.dumps(profile,indent=2),encoding='utf-8')
    # OpenSSL signs a leaf for this fixture only. No OS/browser trust-store change.
    call(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','3','-keyout',str(RUN/'ca.key'),'-out',str(RUN/'ca.crt'),'-subj','/CN=DPP 1A isolated test CA','-addext','basicConstraints=critical,CA:TRUE','-addext','keyUsage=critical,keyCertSign,cRLSign'])
    call(['openssl','req','-newkey','rsa:2048','-nodes','-keyout',str(RUN/'server.key'),'-out',str(RUN/'server.csr'),'-subj','/CN=localhost'])
    (RUN/'leaf.ext').write_text('subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n',encoding='utf-8')
    call(['openssl','x509','-req','-in',str(RUN/'server.csr'),'-CA',str(RUN/'ca.crt'),'-CAkey',str(RUN/'ca.key'),'-CAcreateserial','-days','3','-out',str(RUN/'server.crt'),'-extfile',str(RUN/'leaf.ext')])
    realm={'realm':'dpp-acceptance-1a','enabled':True,'registrationAllowed':False,'resetPasswordAllowed':False,'sslRequired':'all','accessTokenLifespan':600,'ssoSessionIdleTimeout':1200,'users':[{'id':p['subject'],'username':name,'enabled':True,'emailVerified':True,'firstName':name,'lastName':'IsolatedFixture','email':name+'@fixture.invalid','credentials':[{'type':'password','value':p['password'],'temporary':False}]} for name,p in profile['people'].items()],
      'clientScopes':[{'name':'business-acceptance','protocol':'openid-connect','attributes':{'include.in.token.scope':'true'},'protocolMappers':[{'name':'acceptance-audience','protocol':'openid-connect','protocolMapper':'oidc-audience-mapper','config':{'included.client.audience':'dpp-acceptance-api','access.token.claim':'true','id.token.claim':'false'}}]}],
      'clients':[{'clientId':'dpp-acceptance-rp','secret':profile['rpSecret'],'publicClient':False,'standardFlowEnabled':True,'directAccessGrantsEnabled':False,'serviceAccountsEnabled':False,'redirectUris':[ORIGIN+'/auth/callback'],'webOrigins':[ORIGIN],'attributes':{'pkce.code.challenge.method':'S256','post.logout.redirect.uris':ORIGIN+'/auth/logged-out'},'defaultClientScopes':['business-acceptance'],'fullScopeAllowed':False},{'clientId':'dpp-acceptance-api','secret':profile['resourceSecret'],'publicClient':False,'standardFlowEnabled':False,'directAccessGrantsEnabled':False,'serviceAccountsEnabled':True}]}
    (RUN/'realm.json').write_text(json.dumps(realm),encoding='utf-8')
    (RUN/'pg.env').write_text('POSTGRES_PASSWORD='+profile['pgPassword']+'\nPOSTGRES_DB='+DATABASE+'\n',encoding='utf-8')
    (RUN/'idp.env').write_text('KC_BOOTSTRAP_ADMIN_USERNAME=fixture-admin\nKC_BOOTSTRAP_ADMIN_PASSWORD='+profile['adminPassword']+'\n',encoding='utf-8')
    call(['docker','run','-d','--name',PG,'--label','codex.scope=acceptance-identity-1a','--env-file',str(RUN/'pg.env'),'-p','127.0.0.1:25495:5432','--mount',f'type=bind,source={RUN},target=/fixture,readonly','postgres:17-alpine','sh','-c','cp /fixture/server.key /tmp/fixture-server.key && cp /fixture/server.crt /tmp/fixture-server.crt && chown postgres:postgres /tmp/fixture-server.* && chmod 600 /tmp/fixture-server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/fixture-server.crt -c ssl_key_file=/tmp/fixture-server.key'])
    # Match the Linux host file owner, preserving private key mode 0600.
    # The official image's data/conf directories support arbitrary nonroot UID
    # in group 0. Windows bind mounts do not use host POSIX uid ownership.
    user=['--user',str(os.getuid())+':0'] if hasattr(os,'getuid') and os.getuid()!=0 else []
    call(['docker','run','-d','--name',IDP,'--label','codex.scope=acceptance-identity-1a',*user,'--env-file',str(RUN/'idp.env'),'-p','127.0.0.1:19120:8443','--mount',f'type=bind,source={RUN}/server.crt,target=/tls/server.crt,readonly','--mount',f'type=bind,source={RUN}/server.key,target=/tls/server.key,readonly','--mount',f'type=bind,source={RUN}/realm.json,target=/opt/keycloak/data/import/dpp-acceptance-1a-realm.json,readonly',KEYCLOAK,'start-dev','--http-enabled=false','--hostname=https://localhost:19120','--https-certificate-file=/tls/server.crt','--https-certificate-key-file=/tls/server.key','--import-realm'])
    (RUN/'resources.json').write_text(json.dumps({'postgres':metadata(PG),'keycloak':metadata(IDP)},indent=2),encoding='utf-8')
    verify_retained(before,RUN/'retained-comparison.json',observer=metadata)
    print('Isolated 1A PostgreSQL/Keycloak created; credentials remain in the ignored private profile. Retained original unchanged.')
if __name__=='__main__':main()
