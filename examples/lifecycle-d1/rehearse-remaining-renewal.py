"""R1 actual six-Renew regression on a new isolated PostgreSQL backup restore.

Reuses pg_restore, existing reader Command and immutable backup. No native
storage, gateway, sender, IAM enable or DELETE owner is started. Keep all data.
"""
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid
from urllib.parse import urlsplit, urlunsplit

spec=importlib.util.spec_from_file_location('r1_freezer',Path(__file__).with_name('freeze-remaining-executable.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f);r=f.r
PRIVATE=r.PRIVATE/'remaining-renew-r1-01'
PUBLIC=f.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-renew-r1-01'

def main():
    r.require(not r.call(['git','status','--porcelain']).strip(),'Commit source before native rehearsal')
    r.require(not PRIVATE.exists() and not PUBLIC.exists(),'One-shot rehearsal already allocated')
    PRIVATE.mkdir();PUBLIC.mkdir();owned=[];failure=None
    source=r.call(['git','rev-parse','HEAD']).decode().strip()
    packet=f.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate10'
    g=next(g for g in r.load(packet/'candidate.json')['Groups'] if g['Group']=='business-races')
    backup=r.PRIVATE/'remaining-executable03/business-races/checkpoint'
    for name,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256')]:r.require(r.sha(backup/name)==g[key],'Retained backup changed')
    for name in ('Core','Native','Gateway'):r.require(not r.inspect(g[name])['State']['Running'],'Source active')
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    build=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(PRIVATE/'reader'),'./cmd/lifecycle-fixture-reader'],cwd=f.ROOT/'apps/platform',env=env,capture_output=True)
    r.require(build.returncode==0,'R1 reader build failed')
    prefix='dpp307-renew-r1-01';network=prefix+'-private'
    r.docker('network','create','--internal','--label','lifecycle307.technical=renew-r1',network)
    r.docker('volume','create','--label','lifecycle307.technical=renew-r1',prefix+'-core')
    (PRIVATE/'password').write_text(secrets.token_hex(32),encoding='utf-8')
    try:
        pg=r.docker('create','--pull','never','--name',prefix+'-pg','--label','lifecycle307.technical=renew-r1','--network',network,'--network-alias','dpp-lifecycle307-808a6960-candidate07-checkpoint','--mount','type=volume,source='+prefix+'-core,target=/var/lib/postgresql/data','--mount',f'type=bind,source={backup},target=/run/backup,readonly','--mount',f'type=bind,source={PRIVATE}/password,target=/run/password,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/password',r.PG).decode().strip();owned.append(pg);r.docker('start',pg)
        for _ in range(100):
            if b'PostgreSQL init process complete; ready for start up.' in f.docker_logs(pg):break
            time.sleep(.5)
        else:raise ValueError('Clone PG initialization timed out')
        time.sleep(1);r.native.wait_pg(pg,'checkpoint_admin')
        r.docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
        r.docker('exec',pg,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
        before=r.native.fingerprints(pg);r.require(before==r.load(packet/'business-races/core-readonly.json'),'Full clone fingerprint differs')
        r.write(PUBLIC/'restored-core-before.json',before)
        heads=r.load(f.PREP/'business-races/policy-heads.json')
        query="SELECT coalesce(json_agg(row_to_json(p) ORDER BY p.id),'[]') FROM lifecycle_copy_policy p;"
        previous=json.loads(r.native.sql(pg,query));r.write(PUBLIC/'old-policies.json',previous)
        old=r.PRIVATE/'remaining-executable03/business-races/source-owner'
        template=r.load(old/'remaining.json')
        cfg=f.window.case_profile(template,heads,secrets.token_hex(32),template['Base']['Epoch'])
        cfg.update(Rehearsal=True,Checkpoint='candidate07',Batch=str(uuid.uuid4()))
        at=r.utc();cfg['Base'].update(StartedAt=at.isoformat(),ExpiresAt=(at+dt.timedelta(minutes=55)).isoformat())
        dsn=urlsplit(cfg['Base']['PostgresDSN']);cfg['Base']['PostgresDSN']=urlunsplit(dsn._replace(netloc=dsn.netloc.rsplit('@',1)[0]+'@dpp-lifecycle307-808a6960-candidate07-checkpoint:5432'))
        owner=PRIVATE/'owner';owner.mkdir();r.write(owner/'remaining.json',cfg);(owner/'manifest.json').write_bytes((old/'manifest.json').read_bytes())
        output=PUBLIC/'renew';output.mkdir()
        helper=r.docker('create','--pull','never','--name',prefix+'-owner','--label','lifecycle307.technical=renew-r1','--network','container:'+pg,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={owner},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode','remaining-freeze-policy').decode().strip();owned.append(helper)
        r.docker('start',helper);code=int(r.docker('wait',helper));logs=f.docker_logs(helper);(PUBLIC/'renew.stdout.txt').write_bytes(logs)
        r.require(code==0,'Actual clone Renew failed; retained, no retry')
        result=r.load(output/'candidate07-copy-policies.json');r.require(len(result['facts'])==6 and result['delete']==0,'Six actual Renew facts missing')
        after=json.loads(r.native.sql(pg,query));old_by_id={x['id']:x for x in previous};after_by_id={x['id']:x for x in after}
        r.require(all(after_by_id.get(k)==v for k,v in old_by_id.items()) and len(after)==len(previous)+6,'Old policy facts changed or six appends missing')
        r.write(PUBLIC/'policies-after.json',after)
        r.write(PUBLIC/'acceptance.json',{'status':'PASS_SIX_RENEW_ISOLATED_PG','source':source,'binary_sha256':r.sha(PRIVATE/'reader'),'clone_core':pg,'owner':helper,'execution_is_rehearsal_only':cfg['Batch'],'candidate_execution':f.EXECUTION,'CandidateUseUntil':cfg['CandidateUseUntil'],'window_end':f.END,'technical_base_end':cfg['Base']['ExpiresAt'],'policy_use_until':result['use_until'],'six_appends':6,'old_facts_retained':True,'native_storage_started':False,'provider_io':0,'native_delete':0,'send_authority':'NONE'})
    except Exception as error:
        failure=error;r.write(PUBLIC/'failure.json',{'error':str(error),'native_delete':0});raise
    finally:
        closed=f.close_known(owned);r.write(PUBLIC/'physical-close.json',closed)
        if not closed['all_closed'] and failure is None:raise RuntimeError('Clone closure unconfirmed')
    r.write(PUBLIC/'evidence-index.json',{'files':{p.relative_to(f.ROOT).as_posix():r.sha(p) for p in PUBLIC.rglob('*') if p.is_file()}})
    print(json.dumps(r.load(PUBLIC/'acceptance.json')))

if __name__=='__main__':main()
