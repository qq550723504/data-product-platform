"""One-shot remaining-D1 native preparation. No send/enable/approve/delete mode.

Uses existing native provisioner and OSS dump/restore/tar. Failure preserves all
resources. Only resources allocated by this invocation are stopped in finally.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import secrets
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private')
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-native01'
PGIMAGE = 'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
MIIMAGE = 'sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
GROUPS = ('business-races', 'late-hold', 'mixed')


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def write(path, value):
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write('\n')


def call(args, data=None, output=None):
    if output:
        with output.open('xb') as stream:
            result = subprocess.run(list(map(str, args)), input=data, stdout=stream, stderr=subprocess.PIPE, cwd=ROOT)
    else:
        result = subprocess.run(list(map(str, args)), input=data, capture_output=True, cwd=ROOT)
    if result.returncode:
        log = PRIVATE / 'remaining-native01' / ('failure-' + dt.datetime.now(dt.timezone.utc).strftime('%H%M%S%f') + '.private-log')
        log.write_bytes((result.stdout or b'') + b'\n' + result.stderr)
        raise RuntimeError('Controlled command failed; private diagnostics and resources retained: ' + log.name)
    return result.stdout or b''


def docker(*args, **kwargs):
    return call(['docker', *args], **kwargs)


def inspect(name):
    return json.loads(docker('inspect', name))[0]


def sql(container, query):
    return docker('exec', '-i', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'fixture', '-d', 'lifecycle307',
                  data=('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL timezone=\'UTC\';\n' + query + '\nROLLBACK;').encode()).decode().strip()


def fingerprints(container):
    spec = importlib.util.spec_from_file_location('retained_core_tools', ROOT / 'examples/lifecycle-d1/complete-lc13-readonly.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.CONTROL = PRIVATE / 'remaining-native01'
    relations = module.fingerprint(container)
    acl = module.role_acl(container)
    # All custom and builtin role attributes/password hashes are private inputs
    # to a public hash, never emitted. The restore-only bootstrap is excluded.
    roles = sql(container, "SELECT coalesce(jsonb_agg(to_jsonb(r)-'oid' ORDER BY rolname),'[]')::text FROM pg_authid r WHERE rolname<>'checkpoint_admin';")
    extra = sql(container, "SELECT jsonb_build_object('schemas',(SELECT jsonb_agg(jsonb_build_array(nspname,pg_get_userbyid(nspowner),nspacl) ORDER BY nspname) FROM pg_namespace WHERE nspname='public'),'functions',(SELECT jsonb_agg(jsonb_build_array(p.oid::regprocedure::text,pg_get_userbyid(p.proowner),p.proacl) ORDER BY p.oid::regprocedure::text) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'),'defaults',(SELECT coalesce(jsonb_agg(jsonb_build_array(pg_get_userbyid(defaclrole),coalesce(n.nspname,''),defaclobjtype,defaclacl) ORDER BY pg_get_userbyid(defaclrole),coalesce(n.nspname,''),defaclobjtype),'[]') FROM pg_default_acl a LEFT JOIN pg_namespace n ON n.oid=a.defaclnamespace))::text;")
    return {'relations': relations, 'roles_acl': acl, 'all_roles_sha256': hashlib.sha256(roles.encode()).hexdigest(),
            'schema_function_default_acl_sha256': hashlib.sha256(extra.encode()).hexdigest()}


def wait_pg(container, user):
    for _ in range(60):
        result = subprocess.run(['docker', 'exec', container, 'pg_isready', '-U', user, '-d', 'postgres'], capture_output=True)
        if result.returncode == 0:
            return
        time.sleep(1)
    raise RuntimeError('Owned checkpoint startup unavailable; no migration or restore retry')


def gateway(manifest, active):
    # This storage route is not a worker/DirectData HTTP authorization boundary.
    # Response loss applies only to H2-A and M1-B; other exact targets get 204.
    rules = ['{\n admin off\n auto_https off\n}\n:9000 {\n route {',
             '  @bypass header X-Amz-Bypass-Governance-Retention *\n  respond @bypass 403',
             '  @copy header X-Amz-Copy-Source *\n  respond @copy 403']
    if active:
        for index, obj in enumerate(manifest['objects']):
            if obj['key'] not in {'fixture/p1.csv','fixture/p2.csv','fixture/t1.csv','fixture/t2.csv','fixture/h2-a.csv','fixture/m1-a.csv','fixture/m1-b.csv'}:
                continue
            uri = '/' + obj['namespace'] + '/' + obj['key'] + '?versionId=' + obj['version_id']
            rules.append(f"  @exact{index} {{\n   method DELETE\n   expression {{http.request.header.Authorization}}.contains('Credential={manifest['iam']['delete_access']}/') && {{http.request.uri}} == '{uri}'\n  }}")
            proxy = f'  reverse_proxy @exact{index} minio:9000'
            if obj['key'] in {'fixture/h2-a.csv','fixture/m1-b.csv'}:
                proxy += ' {\n   @accepted status 204\n   handle_response @accepted {\n    abort\n   }\n  }'
            rules.append(proxy)
    rules.append('  respond 403\n }\n}\n')
    return '\n'.join(rules).encode()


def complete_native(group, private, resources, out, stage, manifest, owned, source_before, ident, volume):
    pg,mi,gw=(resources['containers'][x] for x in ('postgres','minio','gateway'))
    audit=resources['project']+'-checkpoint'
    restorevolume=resources['project']+'-restored-objects'
    cfg=load(private/'config.json')
    compare=resources['project']+'-compare'
    command=r'set -o pipefail; cd /source; find . -type f -exec sha256sum {} \; | sort > /tmp/source; cd /restore; find . -type f -exec sha256sum {} \; | sort > /tmp/restore; cmp /tmp/source /tmp/restore; cat /tmp/source'
    compareid=docker('create','--pull','never','--name',compare,'--label','lifecycle307.run='+resources['run_id'],'--network','none','--read-only','--tmpfs','/tmp',
                     '--mount','type=volume,source='+resources['project']+'-objects,target=/source,readonly',
                     '--mount','type=volume,source='+restorevolume+',target=/restore,readonly','--entrypoint','/bin/sh',PGIMAGE,'-ec',command).decode().strip()
    owned.append(compareid);docker('start',compareid);require(docker('wait',compareid).strip()==b'0','Native restored file SHA256 comparison failed')
    files=docker('logs',compareid)
    require(len(files.splitlines())>=len(manifest['objects']), 'Native physical file manifest incomplete')
    (out/'native-file-sha256.txt').write_bytes(files)
    clone = resources['project']+'-restored-minio'
    cloneid = docker('create','--pull','never','--name',clone,'--label','lifecycle307.run='+resources['run_id'],'--network','none',
                     '--mount','type=volume,source='+restorevolume+',target=/data',
                     '--mount',f'type=bind,source={private},target=/run/credentials,readonly','--entrypoint','/bin/sh',MIIMAGE,'/run/credentials/start.sh').decode().strip()
    owned.append(cloneid);docker('start',cloneid)
    # Readiness is only a startup check, not a retry of any fixture request.
    for _ in range(60):
        logs=docker('logs',cloneid)
        if b'API:' in logs: break
        time.sleep(1)
    else: raise RuntimeError('Restored native startup not witnessed')
    verifier = resources['project']+'-readonly-restore'
    cfg['OutputDirectory']='/run/output/restore-verification'
    write(stage/'verify.json',cfg)
    verifyid=docker('create','--pull','never','--name',verifier,'--label','lifecycle307.run='+resources['run_id'],'--network','container:'+cloneid,'--read-only',
                    '--mount',f'type=bind,source={private},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output',
                    '--mount',f'type=bind,source={out / "manifest.json"},target=/run/manifest.json,readonly',
                    '-e','LIFECYCLE_FIXTURE_CONFIG=/run/private/checkpoint/verify.json','-e','LIFECYCLE_REMAINING_VERIFY=1',
                    '--entrypoint','/run/private/prepare',PGIMAGE).decode().strip()
    owned.append(verifyid);docker('start',verifyid);require(docker('wait',verifyid).strip()==b'0','Native restored read-only verification failed; no repeat')
    require(load(out/'restore-verification/verification.json')['status']=='PASS_NATIVE_READONLY','Restored native verification incomplete')
    print(group+': restored native GET/inventory/protection/IAM matched',flush=True)
    active,sealed=gateway(manifest,True),gateway(manifest,False)
    (out/'gateway-active.Caddyfile').write_bytes(active)
    (out/'gateway-sealed.Caddyfile').write_bytes(sealed)
    (private/'Caddyfile').write_bytes(sealed)
    for path in (out/'gateway-active.Caddyfile',out/'gateway-sealed.Caddyfile'):
        docker('cp',path,gw+':/tmp/'+path.name)
        docker('exec',gw,'caddy','validate','--config','/tmp/'+path.name,'--adapter','caddyfile')
    docker('restart',gw)
    require(fingerprints(pg)==source_before,'Source Core changed during backup/read-only verification')
    write(out/'checkpoint.json',{'status':'PASS_FULL_CORE_AND_NATIVE_RESTORE','core':{'container':audit,'id':ident,'volume':volume,'dump_sha256':sha(stage/'core.dump'),'globals_sha256':sha(stage/'globals.sql'),'tables':len(source_before['relations']),'rows':sum(x['rows'] for x in source_before['relations'])},
                                'native':{'archive_sha256':sha(stage/'native.tar'),'archive_bytes':(stage/'native.tar').stat().st_size,'container':clone,'id':cloneid,'volume':restorevolume,'version_count':len(manifest['objects']),'physical_file_sha256_compare':'PASS','exact_GET_inventory_protection_IAM':'PASS'},
                                'active_gateway_sha256':hashlib.sha256(active).hexdigest(),'sealed_gateway_sha256':sha(private/'Caddyfile'),'installed_gateway':'SEALED_DENY_ALL','delete':0,'hold_release':0,'bypass':0,'cleanup':0})


def prepare(group, subnet):
    control = PRIVATE / 'remaining-native01'
    out = PUBLIC / group
    require(not (control / (group + '.json')).exists() and not out.exists(), 'One-shot group already allocated')
    out.mkdir()
    before = set(PRIVATE.glob('dpp-lifecycle307-*'))
    owned = []
    facts = None
    try:
        call(['pwsh','-NoProfile','-File',ROOT/'examples/lifecycle-d1/prepare.ps1','-PrivateRoot',PRIVATE,
              '-ArchiveAcceptance','-ProcessingDeliveryAcceptance','-RemainingGroup',group,'-InternalSubnet',subnet,'-MinioImage',MIIMAGE], output=out/'provision.stdout.txt')
        created = set(PRIVATE.glob('dpp-lifecycle307-*')) - before
        require(len(created) == 1, 'Ambiguous new native directory')
        private = created.pop()
        resources = load(private/'resources.json')
        cfg = load(private/'config.json')
        owned.extend(resources['containers'].values())
        require(not cfg['Enabled'] and cfg['RemainingGroup'] == group, 'Preparation authority not disabled')
        resultdir = ROOT/'examples/lifecycle-d1/results'/resources['run_id']
        manifest = load(resultdir/'manifest.json')
        print(group+': native provision complete',flush=True)
        write(out/'manifest.json', manifest)
        write(control/(group+'.json'), {'private':str(private),'resources':resources})
        # Full retained binaries/source binding are collected by finalize after
        # every group has succeeded. These new service IDs never name old runs.
        stage = private/'checkpoint'
        stage.mkdir()
        pg, mi, gw = (resources['containers'][x] for x in ('postgres','minio','gateway'))
        source_before = fingerprints(pg)
        write(out/'source-before.json', source_before)
        require(sql(pg,'SELECT max(version) FROM schema_migration;') == '68', 'Current schema drift')
        require(sql(pg,'SELECT (SELECT count(*) FROM lifecycle_disposition_approval),(SELECT count(*) FROM lifecycle_provider_attempt),(SELECT count(*) FROM lifecycle_disposition_dispatch),(SELECT count(*) FROM lifecycle_reader_pin);') == '0|0|0|0', 'Non-delete preparation has mutable execution facts')
        docker('exec',pg,'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create',output=stage/'core.dump')
        docker('exec',pg,'pg_dumpall','-U','fixture','--globals-only',output=stage/'globals.sql')
        (stage/'password').write_text(secrets.token_hex(32),encoding='ascii')
        audit = resources['project']+'-checkpoint'
        volume = audit+'-data'
        docker('volume','create','--label','lifecycle307.run='+resources['run_id'],volume)
        ident = docker('create','--pull','never','--name',audit,'--label','lifecycle307.run='+resources['run_id'],'--network','none',
                       '--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data',
                       '--mount',f'type=bind,source={stage},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres',
                       '-e','POSTGRES_PASSWORD_FILE=/run/backup/password',PGIMAGE).decode().strip()
        owned.append(ident)
        docker('start',ident)
        wait_pg(ident,'checkpoint_admin')
        docker('exec',ident,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
        docker('exec',ident,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
        restored_facts=fingerprints(ident)
        write(out/'restored-core.json',restored_facts)
        require(restored_facts == source_before, 'Full restored Core rows/roles/ACL differ')
        print(group+': full Core restore matched',flush=True)
        # Freeze source storage while taking a physical named-volume archive.
        docker('stop','--time','20',mi)
        archive = resources['project']+'-archive'
        archiveid = docker('create','--pull','never','--name',archive,'--label','lifecycle307.run='+resources['run_id'],'--network','none','--read-only',
                           '--mount','type=volume,source='+resources['project']+'-objects,target=/source,readonly','--mount',f'type=bind,source={stage},target=/backup',
                           '--entrypoint','/bin/sh',PGIMAGE,'-c','exec tar -C /source -cpf /backup/native.tar .').decode().strip()
        owned.append(archiveid)
        docker('start',archiveid);require(docker('wait',archiveid).strip()==b'0','Native archive failed')
        restorevolume = resources['project']+'-restored-objects'
        docker('volume','create','--label','lifecycle307.run='+resources['run_id'],restorevolume)
        unpack = resources['project']+'-unpack'
        unpackid = docker('create','--pull','never','--name',unpack,'--label','lifecycle307.run='+resources['run_id'],'--network','none','--read-only',
                          '--mount','type=volume,source='+restorevolume+',target=/restore','--mount',f'type=bind,source={stage},target=/backup,readonly',
                          '--entrypoint','/bin/sh',PGIMAGE,'-c','exec tar -C /restore -xpf /backup/native.tar').decode().strip()
        owned.append(unpackid);docker('start',unpackid);require(docker('wait',unpackid).strip()==b'0','Native restore failed')
        complete_native(group,private,resources,out,stage,manifest,owned,source_before,ident,volume)
        facts={'group':group,'run_id':resources['run_id'],'status':'PASS_PREPARATION_ONLY','checkpoint':'checkpoint.json','current_delete_budget':0}
    finally:
        # Recover allocation identity even if native provision failed. No retry,
        # removal, down -v, or action against preexisting project names.
        for private in set(PRIVATE.glob('dpp-lifecycle307-*'))-before:
            if (private/'resources.json').exists():
                allocated=load(private/'resources.json')
                owned.extend(allocated['containers'].values())
                owned.extend(docker('ps','-aq','--filter','label=lifecycle307.run='+allocated['run_id']).decode().split())
                configpath=private/'config.json'
                if configpath.exists():
                    state=load(configpath)
                    if state['Enabled']:
                        write(private/'failed-preparation-config.json',state)
                        state['Enabled']=False
                        configpath.write_text(json.dumps(state,indent=2)+'\n',encoding='utf-8')
        seals=[]
        for name in dict.fromkeys(owned):
            item=inspect(name)
            if item['State']['Running']:docker('stop','--time','20',item['Id'])
            item=inspect(item['Id'])
            require(not item['State']['Running'] and item['State']['Pid']==0 and not any(item['NetworkSettings']['Ports'].values()),'Owned resource not sealed')
            seals.append({'name':item['Name'],'id':item['Id'],'image':item['Image'],'status':item['State']['Status'],'pid':item['State']['Pid'],'ports':item['NetworkSettings']['Ports'],'mounts':item['Mounts']})
        write(out/'seal.json',{'containers':seals,'cleanup':0,'delete':0,'retained':True})
        if facts:write(out/'preparation.json',facts)


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('group',choices=GROUPS)
    parser.add_argument('--subnet',required=True)
    args=parser.parse_args()
    require((PRIVATE/'remaining-native01').is_dir(),'Create restricted private control directory first')
    require(not call(['git','status','--porcelain','--untracked-files=no']).strip(),'Commit tracked source before native preparation')
    newpaths=call(['git','ls-files','--others','--exclude-standard','apps/platform','examples/lifecycle-d1']).decode().splitlines()
    require(not any(Path(p).suffix in {'.go','.py','.ps1','.Caddyfile','.sql'} for p in newpaths),'Commit all candidate source including new tools before preparation')
    PUBLIC.mkdir(exist_ok=True)
    prepare(args.group,args.subnet)
