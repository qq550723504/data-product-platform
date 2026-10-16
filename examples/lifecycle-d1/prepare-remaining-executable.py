"""One-shot candidate07 preparation. Native DELETE, cleanup and send enable: NONE.

Append future-window business policies/rights on the three fixed v16 sources,
verify exact native inventory and disabled IAM, take full Core/native backups,
restore each into new isolated volumes, and rerun business/C protection there.
Every failure and resource is retained. No old checkpoint is overwritten.
"""
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time
import uuid
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('runner', Path(__file__).with_name('prepare-remaining-runner.py'))
r = importlib.util.module_from_spec(spec); spec.loader.exec_module(r)
require, load, write, sha, docker, inspect = r.require, r.load, r.write, r.sha, r.docker, r.inspect
CONTROL = r.PRIVATE / 'remaining-executable01'
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-executable01'
END = '2026-10-17T02:40:00Z'
EXECUTION = 'effa0385-9308-42eb-bd66-d0c379b6a989'


def main():
    require(not r.call(['git', 'status', '--porcelain']).strip(), 'Commit source before one-shot preparation')
    require(not CONTROL.exists() and not PUBLIC.exists(), 'Preparation already allocated; retain and diagnose failures')
    CONTROL.mkdir(); PUBLIC.mkdir()
    source = r.call(['git', 'rev-parse', 'HEAD']).decode().strip()
    env = os.environ.copy(); env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    for name, package in [('reader', 'lifecycle-fixture-reader'), ('operator', 'lifecycle-fixture-remaining')]:
        result = subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-o', str(CONTROL/name), './cmd/'+package], cwd=ROOT/'apps/platform', env=env, capture_output=True)
        require(result.returncode == 0, name+' build failed')
    write(PUBLIC/'build.json', {'source':source, 'reader_sha256':sha(CONTROL/'reader'), 'operator_sha256':sha(CONTROL/'operator'), 'go':r.call(['go','version']).decode().strip(), 'native_delete':0})
    scope = load(ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
    for index, fixed in enumerate(scope):
        group = fixed['Group']; run = fixed['Run']; project = 'dpp-lifecycle307-'+run[:8]
        original = r.PRIVATE/project; original_cfg = load(original/'config.json'); resources = load(original/'resources.json')
        historical = ROOT/'docs/acceptance/evidence/issue307-d1/remaining-native01'/group
        manifest = load(historical/'manifest.json')
        target = PUBLIC/group; target.mkdir(); private = CONTROL/group; private.mkdir()
        owned = []; prefix = project+'-candidate07'; checkpoint = private/'checkpoint'; checkpoint.mkdir()
        network = prefix+'-private'; subnet = '10.234.'+str(172+index)+'.0/24'; r.check_subnet(subnet)
        source_ids = {k:inspect(v)['Id'] for k,v in resources['containers'].items()}
        expected_ids = {k:next(c['id'] for c in manifest['environment']['containers'] if c['name']==resources['containers'][k]) for k in source_ids}
        require(source_ids == expected_ids, 'Source physical IDs drifted')
        for ident in source_ids.values():
            current = inspect(ident)
            require(not current['State']['Running'] and current['State']['Pid'] == 0 and not any(current['NetworkSettings']['Ports'].values()), 'Source active or exposed')
        require(inspect(source_ids['minio'])['Image'] == r.MI and inspect(source_ids['postgres'])['Image'] == r.PG, 'Source images drifted')
        base = {k:original_cfg[k] for k in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','PostgresDSN','InspectAccess','InspectSecret')}
        base.update(Enabled=True, Epoch=str(uuid.uuid4()), Token=secrets.token_hex(32), Access=base['InspectAccess'], Secret=base['InspectSecret'], OutputDirectory='/run/output', ManifestSHA256=sha(historical/'manifest.json'), StartedAt=r.utc().isoformat(), ExpiresAt=(r.utc()+dt.timedelta(minutes=55)).isoformat())
        if group == 'business-races':
            prior = load(r.PRIVATE/'remaining-runner02/owner/remaining.json')['Base']
            for key in ('Access','Secret','ProducerAccess','ProducerSecret'): base[key] = prior[key]
            base['RedisAddr'] = project+'-remaining-redis:6379'
        conf = {'Base':base,'Group':group,'Batch':EXECUTION,'NonDelete':True,'CandidateUseUntil':END}
        owner = private/'source-owner'; admin = private/'source-admin'; owner.mkdir(); admin.mkdir()
        for p in (owner, admin): (p/'manifest.json').write_bytes((historical/'manifest.json').read_bytes())
        write(owner/'remaining.json', conf)
        root_conf = json.loads(json.dumps(conf)); root_conf['Base'].update(RootAccess=original_cfg['RootAccess'], RootSecret=original_cfg['RootSecret'])
        write(admin/'remaining.json', root_conf)

        def helper(name, mode, cfgdir, netns, output):
            output.mkdir(exist_ok=True)
            ident = docker('create','--pull','never','--name',prefix+'-'+name,'--label','lifecycle307.remaining='+EXECUTION,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+netns,'--mount',f'type=bind,source={cfgdir},target=/run/private,readonly','--mount',f'type=bind,source={CONTROL}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={ROOT}/industry-packs,target=/run/repo/industry-packs,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip()
            owned.append(ident); docker('start',ident); code=int(docker('wait',ident))
            (target/(name+'.stdout.txt')).write_bytes(docker('logs',ident))
            write(target/(name+'.json'),{'id':ident,'mode':mode,'exit_code':code,'binary_sha256':sha(CONTROL/'reader'),'native_delete':0})
            require(code == 0, name+' failed; do not blindly rerun')
            return ident

        def utility(name, command, mounts):
            ident=docker('create','--pull','never','--name',prefix+'-'+name,'--label','lifecycle307.remaining='+EXECUTION,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp',*mounts,'--entrypoint','/bin/sh',r.PG,'-ec',command).decode().strip()
            owned.append(ident);docker('start',ident);require(docker('wait',ident).strip()==b'0',name+' failed');return ident

        try:
            for key in ('postgres','minio'): owned.append(source_ids[key]);docker('start',source_ids[key])
            r.native.wait_pg(source_ids['postgres'],'fixture');time.sleep(3)
            # Compile all future operator SQL against the actual schema without
            # executing a query, claiming work, or contacting native storage.
            sqls=[]
            for path in (ROOT/'apps/platform/cmd/lifecycle-fixture-remaining').glob('*.go'):
                if path.name.endswith('_test.go'): continue
                for query in re.findall(r'`(SELECT [^`]+)`',path.read_text()): sqls.append(query)
            for i,query in enumerate(sqls): r.native.sql(source_ids['postgres'],f'PREPARE remaining_{i} AS {query}; DEALLOCATE remaining_{i};')
            write(target/'future-sql-prepared.json',{'queries':len(sqls),'transaction':'READ ONLY; PREPARE/DEALLOCATE/ROLLBACK','execution':0})
            if group == 'business-races':
                helper('freeze-policy','remaining-freeze-policy',owner,source_ids['minio'],target/'policy')
                helper('bind-outputs','remaining-bind-outputs',admin,source_ids['minio'],target/'policy-native')
            query="SELECT json_agg(json_build_object('ID',c.id,'WorkspaceID',c.workspace_id,'ProviderInstanceID',n.provider_instance_id,'IncarnationID',n.incarnation_id,'Namespace',n.namespace,'Key',c.object_key,'ProviderVersion',c.provider_version,'DatasetVersionID',c.dataset_version_id,'ContentSHA256',c.content_sha256,'ByteSize',c.byte_size) ORDER BY c.object_key) FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id;"
            copies = json.loads(r.native.sql(source_ids['postgres'], query))
            require(len(copies) == (8 if group=='business-races' else 6 if group=='late-hold' else 4),'Exact inventory count differs')
            protections = {o['copy_id']:o for o in manifest['objects']}
            expected=[]
            for cp in copies:
                old=protections.get(cp['ID'])
                expected.append({'Copy':cp,'LegalHold':old['legal_hold'] if old else False,'Retention':old['retention_mode'] if old else 'NONE','RetainUntil':old['retain_until'] if old else None})
            write(admin/'inventory.json',expected);root_conf['InventorySHA256']=sha(admin/'inventory.json');write(admin/'remaining.json',root_conf)
            helper('source-readonly','remaining-verify-native',admin,source_ids['minio'],target/'source-readonly')
            facts = json.loads(r.native.sql(source_ids['postgres'],"SELECT json_build_object('approvals',(SELECT count(*) FROM lifecycle_disposition_approval),'dispatches',(SELECT count(*) FROM lifecycle_disposition_dispatch),'attempts',(SELECT count(*) FROM lifecycle_provider_attempt),'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'restrictions',(SELECT count(*) FROM lifecycle_content_use_restriction),'unclosed_pins',(SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)));"))
            require(all(v == 0 for v in facts.values()),'Source execution already advanced')
            write(target/'source-facts.json',facts)
            before=r.native.fingerprints(source_ids['postgres']);write(target/'source-core.json',before)
            (checkpoint/'password').write_text(secrets.token_hex(32))
            docker('exec',source_ids['postgres'],'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create',output=checkpoint/'core.dump')
            docker('exec',source_ids['postgres'],'pg_dumpall','-U','fixture','--globals-only',output=checkpoint/'globals.sql')
            docker('stop','--time','20',source_ids['minio'])
            utility('archive','exec tar -C /source -cpf /backup/native.tar .',['--mount','type=volume,source='+project+'-objects,target=/source,readonly','--mount',f'type=bind,source={checkpoint},target=/backup'])
            docker('network','create','--internal','--subnet',subnet,'--label','lifecycle307.remaining='+EXECUTION,network)
            for suffix in ('core','objects','redis'):docker('volume','create','--label','lifecycle307.remaining='+EXECUTION,prefix+'-'+suffix)
            pg=docker('create','--pull','never','--name',prefix+'-checkpoint','--label','lifecycle307.remaining='+EXECUTION,'--network',network,'--mount','type=volume,source='+prefix+'-core,target=/var/lib/postgresql/data','--mount',f'type=bind,source={checkpoint},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/backup/password',r.PG).decode().strip();owned.append(pg);docker('start',pg)
            ready=False
            for _ in range(80):
                if b'PostgreSQL init process complete; ready for start up.' in docker('logs',pg): ready=True;break
                time.sleep(.5)
            require(ready,'Final PG init marker missing');time.sleep(1);r.native.wait_pg(pg,'checkpoint_admin')
            docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
            docker('exec',pg,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
            restored=r.native.fingerprints(pg);require(restored==before,'Core rows/roles/ACL restore mismatch');write(target/'restored-core.json',restored)
            utility('unpack','exec tar -C /restore -xpf /backup/native.tar .',['--mount','type=volume,source='+prefix+'-objects,target=/restore','--mount',f'type=bind,source={checkpoint},target=/backup,readonly'])
            compare=utility('compare',r'''set -o pipefail
cd /source
find . -type f -exec sha256sum {} \; | sort > /tmp/source
cd /restore
find . -type f -exec sha256sum {} \; | sort > /tmp/restore
cmp /tmp/source /tmp/restore
cat /tmp/source''',['--mount','type=volume,source='+project+'-objects,target=/source,readonly','--mount','type=volume,source='+prefix+'-objects,target=/restore,readonly'])
            (target/'native-file-sha256.txt').write_bytes(docker('logs',compare))
            mi=docker('create','--pull','never','--name',prefix+'-minio','--label','lifecycle307.remaining='+EXECUTION,'--network',network,'--network-alias','minio','--mount','type=volume,source='+prefix+'-objects,target=/data','--mount',f'type=bind,source={original},target=/run/credentials,readonly','--entrypoint','/bin/sh',r.MI,'/run/credentials/start.sh').decode().strip();owned.append(mi);docker('start',mi);time.sleep(3)
            clone_conf=json.loads(json.dumps(conf));clone_conf.update(Rehearsal=True,Checkpoint='candidate07',Batch=str(uuid.uuid4()))
            dsn=urlsplit(base['PostgresDSN']);clone_conf['Base']['PostgresDSN']=urlunsplit(dsn._replace(netloc=dsn.netloc.rsplit('@',1)[0]+'@'+prefix+'-checkpoint:5432'))
            clone=private/'clone-owner';clone_admin=private/'clone-admin';clone.mkdir();clone_admin.mkdir()
            for p in (clone,clone_admin):(p/'manifest.json').write_bytes((historical/'manifest.json').read_bytes())
            write(clone/'remaining.json',clone_conf)
            ca=json.loads(json.dumps(clone_conf));ca['Base'].update(RootAccess=original_cfg['RootAccess'],RootSecret=original_cfg['RootSecret']);ca['InventorySHA256']=root_conf['InventorySHA256'];write(clone_admin/'remaining.json',ca);(clone_admin/'inventory.json').write_bytes((admin/'inventory.json').read_bytes())
            helper('restored-readonly','remaining-verify-native',clone_admin,mi,target/'restored-readonly')
            gateway=private/'Caddyfile'
            if group=='business-races': gateway.write_bytes(r.gateway(base,copies))
            else: gateway.write_text("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n @metadata {\n method GET HEAD\n expression {http.request.header.Authorization}.contains('Credential="+base['InspectAccess']+"/')\n }\n reverse_proxy @metadata minio:9000\n respond 403\n }\n}\n")
            gw=docker('create','--pull','never','--name',prefix+'-gateway','--label','lifecycle307.remaining='+EXECUTION,'--network',network,'--mount',f'type=bind,source={gateway},target=/etc/caddy/Caddyfile,readonly',r.GW,'caddy','run','--config','/etc/caddy/Caddyfile').decode().strip();owned.append(gw);docker('start',gw);time.sleep(1)
            regression=target/'regression';regression.mkdir()
            if group=='business-races':
                (regression/'remaining-state.json').write_bytes((ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner02/business/remaining-state.json').read_bytes())
                redis=docker('create','--pull','never','--name',prefix+'-redis','--label','lifecycle307.remaining='+EXECUTION,'--network',network,'--network-alias',project+'-remaining-redis','--mount','type=volume,source='+prefix+'-redis,target=/data',r.REDIS,'redis-server','--appendonly','yes').decode().strip();owned.append(redis);docker('start',redis)
                helper('clone-enable','remaining-clone-enable',clone_admin,mi,target/'clone-admin-enable')
                helper('business','remaining-business',clone,gw,regression)
                helper('clone-seal','remaining-seal',clone_admin,mi,target/'clone-admin-seal')
            elif group=='mixed':
                helper('clone-inspect-enable','remaining-inspection-enable',clone_admin,mi,target/'clone-admin-enable')
                helper('C-protected','remaining-mixed-protected',clone,gw,regression)
                helper('clone-inspect-disable','remaining-inspection-disable',clone_admin,mi,target/'clone-admin-seal')
            helper('final-restored-readonly','remaining-verify-native',clone_admin,mi,target/'final-restored-readonly')
            write(target/'regression-final-core.json',r.native.fingerprints(pg))
            write(target/'checkpoint.json',{'status':'PASS_FULL_CORE_NATIVE_RESTORE_AND_NONDELETE','source':source,'source_ids':source_ids,'restored_core':pg,'restored_native':mi,'restored_gateway':gw,'network':json.loads(docker('network','inspect',network))[0]['Id'],'subnet':subnet,'core_dump_sha256':sha(checkpoint/'core.dump'),'globals_sha256':sha(checkpoint/'globals.sql'),'native_archive_sha256':sha(checkpoint/'native.tar'),'inventory_sha256':sha(admin/'inventory.json'),'copies':copies,'native_delete':0,'cleanup':0,'source_execution_started':False})
        finally:
            seals=[]
            for ident in dict.fromkeys(reversed(owned)):
                state=inspect(ident)
                if state['State']['Running']:docker('stop','--time','20',ident)
                state=inspect(ident);require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Resource seal failed')
                seals.append({'id':state['Id'],'name':state['Name'],'image':state['Image'],'pid':state['State']['Pid'],'status':state['State']['Status'],'ports':state['NetworkSettings']['Ports']})
            write(target/'seal.json',{'containers':seals,'retained':True,'native_delete':0,'cleanup':0})
        print(json.dumps({'group':group,'restore':'PASS','native_delete':0}),flush=True)


if __name__ == '__main__':main()
