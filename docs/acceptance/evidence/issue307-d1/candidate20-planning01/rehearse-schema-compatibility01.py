"""Restore retained dumps into ONE new network-none PG; never open a source.

The existing migration Runner is exercised only on the independent clone.
No candidate allocation, sender, MinIO, source migration or authorization.
"""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import time
import uuid

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
OUT=HERE/'schema-compatibility01'
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'candidate20-schema-compatibility01'
SOURCE='955e98a2338b7d22a0686866683fb4413bf8dcdd'
EXECUTION='5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6'
PG='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
serial=0

def need(value,message):
    if not value:raise ValueError(message)
def load(path):return json.loads(path.read_bytes())
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def save(path,value):
    with path.open('xb') as stream:stream.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
def raw(path,value):
    with path.open('xb') as stream:stream.write(value)
def call(args,data=None,allow_failure=False,env=None):
    global serial
    serial+=1;prefix=CONTROL/'journal'/f'{serial:03d}'
    save(prefix.with_suffix('.intent.json'),dict(at=dt.datetime.now(dt.timezone.utc).isoformat(),args=list(map(str,args))))
    result=subprocess.run(list(map(str,args)),input=data,capture_output=True,cwd=ROOT,env=env,timeout=300)
    raw(prefix.with_suffix('.stdout.private'),result.stdout);raw(prefix.with_suffix('.stderr.private'),result.stderr)
    save(prefix.with_suffix('.process.json'),dict(exit=result.returncode))
    need(allow_failure or result.returncode==0,'operation failed; private raw evidence '+str(serial))
    return result
def docker(*args,**kwargs):return call(['docker',*args],**kwargs)
def inspect(cid):return json.loads(docker('inspect',cid).stdout)[0]
def closed(item):
    return item['State']['Running'] is False and item['State']['Pid']==0 and item['HostConfig']['RestartPolicy']['Name']=='no' and not item['HostConfig']['PortBindings'] and not item['NetworkSettings']['Ports']
def q(core,db,text,allow_failure=False):
    return docker('exec','-i',core,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d',db,
        data=("BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;SET LOCAL timezone='UTC';\n"+text+'\nROLLBACK;').encode(),allow_failure=allow_failure)
def identifier(text):return '"'+text.replace('"','""')+'"'
def columns(core,db):
    query="SELECT jsonb_object_agg(name,cols)::text FROM (SELECT c.relname name,jsonb_agg(a.attname ORDER BY a.attnum) cols FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid WHERE n.nspname='public' AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) t;"
    return json.loads(q(core,db,query).stdout)
def projected(core,db,old_columns,versions):
    statements=[]
    for table,names in old_columns.items():
        selected=','.join(identifier(x) for x in names)
        where=' WHERE version IN ('+','.join(str(x) for x in versions)+')' if table=='schema_migration' else ''
        table_literal="'"+table.replace("'","''")+"'"
        sql="SELECT jsonb_build_object('relation',"+table_literal+",'rows',count(*),'sha256',encode(sha256(convert_to(coalesce(string_agg(row_json::text,E'\\n' ORDER BY row_json::text),''),'UTF8')),'hex'))::text FROM (SELECT to_jsonb(t) row_json FROM (SELECT "+selected+" FROM public."+identifier(table)+where+") t) j;"
        statements.append(sql)
    rows=[json.loads(x) for x in q(core,db,'\n'.join(statements)).stdout.decode('utf8','strict').splitlines()]
    need(len(rows)==len(old_columns) and {x['relation'] for x in rows}==set(old_columns),'projection membership differs')
    result={x['relation']:{k:v for k,v in x.items() if k!='relation'} for x in rows}
    return result

def main():
    need(not OUT.exists() and not CONTROL.exists(),'one-shot schema rehearsal already exists')
    OUT.mkdir();CONTROL.mkdir();(CONTROL/'journal').mkdir();(CONTROL/'tools').mkdir();(CONTROL/'migrations').mkdir()
    need(call(['git','-c','core.longpaths=true','rev-parse','HEAD']).stdout.decode().strip()==SOURCE,'exact rehearsal source differs')
    candidate=load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate19/candidate.json')
    originals=[g[key] for g in candidate['Groups'] for key in ('Core','Native')]
    need(all(closed(inspect(cid)) for cid in originals),'source must remain stopped')
    repository=(ROOT/'apps/platform/internal/lifecycle/infrastructure/postgres_repository.go').read_text()
    start=repository.index('SELECT c.id,c.workspace_id,c.dataset_version_id')
    query=repository[start:repository.index(chr(96),start)]
    need("c.identity_kind='PROVIDER_VERSION'" in query,'required current repository predicate absent')
    raw(OUT/'actual-repository-query.sql',query.encode())
    migration_files=sorted((ROOT/'migrations').glob('*.up.sql'))
    for path in migration_files:raw(CONTROL/'migrations'/path.name,path.read_bytes())
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    # Existing cmd/migrate is compiled within its module, unchanged.
    env['GIT_CONFIG_COUNT']='1';env['GIT_CONFIG_KEY_0']='core.longpaths';env['GIT_CONFIG_VALUE_0']='true'
    build=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(CONTROL/'tools/migrate'),'./cmd/migrate'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    raw(OUT/'migrate-build.stdout.txt',build.stdout);raw(OUT/'migrate-build.stderr.txt',build.stderr);need(build.returncode==0,'clone migrator build failed')
    save(OUT/'intent.json',dict(source=SOURCE,execution=EXECUTION,scope='ONLY_NEW_ISOLATED_PG_CLONE',source_started=False,
        migrator_sha256=sha(CONTROL/'tools/migrate'),migrations={p.name:sha(p) for p in migration_files},provider_delete=0))
    nonce=uuid.uuid4().hex
    owners=[];name='dpp307-c20-schema-'+nonce[:12];create_attempted=False
    result=dict(source=SOURCE,execution=EXECUTION,allocation_nonce=nonce,groups=[],source_started=False,source_migration=False,native_delete=0,candidate_owners_allocated=0)
    failure=None
    try:
        need(docker('volume','inspect',name,allow_failure=True).returncode!=0 and docker('inspect',name,allow_failure=True).returncode!=0,'new rehearsal name already exists')
        volume=docker('volume','create','--label','lifecycle307.schema-rehearsal='+EXECUTION,'--label','lifecycle307.allocation='+nonce,name).stdout.decode().strip()
        volume_info=json.loads(docker('volume','inspect',volume).stdout)[0]
        need(volume_info['Labels'].get('lifecycle307.allocation')==nonce and volume_info['Driver']=='local' and not volume_info.get('Options'),'new rehearsal volume not owned')
        args=['create','--pull','never','--restart','no','--name',name,'--network','none','--label','lifecycle307.schema-rehearsal='+EXECUTION,
            '--label','lifecycle307.allocation='+nonce,
            '-e','POSTGRES_USER=fixture','-e','POSTGRES_DB=postgres','-e','POSTGRES_HOST_AUTH_METHOD=trust',
            '--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data',
            '--mount',f'type=bind,source={CONTROL}/tools,target=/run/tools,readonly',
            '--mount',f'type=bind,source={CONTROL}/migrations,target=/run/migrations,readonly']
        for g in candidate['Groups']:
            backup=PRIVATE/'remaining-candidate19'/g['Group']/'checkpoint/core.dump'
            need(sha(backup)==g['CoreDumpSHA256'],'original backup differs')
            args+=['--mount',f'type=bind,source={backup},target=/run/{g["Group"]}.dump,readonly']
        create_attempted=True
        core=docker(*args,PG).stdout.decode().strip();owners.append(core)
        save(CONTROL/'allocation.json',dict(cid=core,volume=volume));save(OUT/'allocation.json',dict(cid=core,volume=volume,network='none',ports=[]))
        docker('start',core)
        for _ in range(60):
            if docker('exec',core,'pg_isready','-U','fixture','-d','postgres',allow_failure=True).returncode==0:break
            time.sleep(.5)
        else:raise RuntimeError('new clone PG not ready')
        for index,g in enumerate(candidate['Groups']):
            db='rehearsal'+str(index);folder=OUT/g['Group'];folder.mkdir()
            docker('exec',core,'createdb','-U','fixture',db)
            docker('exec',core,'pg_restore','-U','fixture','-d',db,'--exit-on-error','--no-owner','--no-acl','/run/'+g['Group']+'.dump')
            before_columns=columns(core,db)
            versions=json.loads(q(core,db,"SELECT jsonb_agg(version ORDER BY version)::text FROM schema_migration;").stdout)
            pending=[int(p.name.split('_')[0]) for p in migration_files if int(p.name.split('_')[0]) not in versions]
            before=projected(core,db,before_columns,versions);save(folder/'old-column-projection-before.json',before)
            target=next(cp for cp in g['Copies'] if cp['Key'] in ('fixture/p2.csv','fixture/h2-a.csv','fixture/m1-a.csv'))
            probe='PREPARE current_repository(uuid,uuid[]) AS '+query+";\nEXECUTE current_repository('"+target['WorkspaceID']+"',ARRAY['"+target['ID']+"']::uuid[]);"
            negative=q(core,db,'\\set VERBOSITY verbose\n'+probe,allow_failure=True)
            raw(folder/'required-query-before.stdout.txt',negative.stdout);raw(folder/'required-query-before.stderr.txt',negative.stderr)
            need(negative.returncode!=0 and b'42703' in negative.stderr and b'identity_kind' in negative.stderr,'expected native missing-column rejection not reproduced')
            applied=docker('exec','-e','POSTGRES_DSN=postgres://fixture@127.0.0.1:5432/'+db+'?sslmode=disable',core,'/run/tools/migrate','-direction','up','-dir','/run/migrations')
            raw(folder/'official-migration.stdout.txt',applied.stdout);raw(folder/'official-migration.stderr.txt',applied.stderr)
            after_versions=json.loads(q(core,db,"SELECT jsonb_agg(version ORDER BY version)::text FROM schema_migration;").stdout)
            after=projected(core,db,before_columns,versions);save(folder/'old-column-projection-after.json',after)
            need(before==after,'migration changed original row/old-column facts, including old pins')
            need(sorted(set(after_versions)-set(versions))==pending,'official Runner applied version set differs')
            positive=q(core,db,probe);raw(folder/'required-query-after.stdout.txt',positive.stdout)
            need(target['ID'].encode() in positive.stdout,'current repository query did not return exact retained copy')
            row=dict(group=g['Group'],original_backup_sha256=g['CoreDumpSHA256'],before_versions=versions,actual_pending_versions=pending,
                after_versions=after_versions,missing_column_SQLSTATE='42703',current_query_after='PASS',old_relations=len(before),
                old_rows_columns_and_pins_unchanged=True,source_started=False,source_migration=False)
            save(folder/'result.json',row);result['groups'].append(row)
        result.update(status='PASS_CLONE_SCHEMA_UPGRADE_ONLY_SOURCE_UPGRADE_NOT_AUTHORIZED',volume_retained=volume)
    except Exception as error:
        failure=dict(type=type(error).__name__,message=str(error));result['status']='FAILED_CLONE_REHEARSAL_RETAINED_NO_SOURCE_WRITE'
    finally:
        closure=[];unconfirmed=[]
        if create_attempted and not owners:
            try:
                item=inspect(name)
                need(item['Config']['Labels'].get('lifecycle307.allocation')==nonce,'ambiguous owner not adopted')
                owners.append(item['Id'])
            except Exception as error:unconfirmed.append(dict(name=name,error_type=type(error).__name__))
        for cid in reversed(owners):
            try:docker('stop','--time','15',cid,allow_failure=True)
            except Exception as error:unconfirmed.append(dict(cid=cid,error_type=type(error).__name__))
            try:closure.append(dict(cid=cid,closed=closed(inspect(cid))))
            except Exception as error:closure.append(dict(cid=cid,closed=False,error_type=type(error).__name__))
        source_closure=[]
        for cid in originals:
            try:source_closure.append(dict(cid=cid,closed=closed(inspect(cid))))
            except Exception as error:source_closure.append(dict(cid=cid,closed=False,error_type=type(error).__name__))
        result.update(failure=failure,closure=closure,unconfirmed=unconfirmed,source_closure=source_closure,source_owners_still_closed=all(x['closed'] for x in source_closure))
        if unconfirmed or not all(x['closed'] for x in closure) or not result['source_owners_still_closed']:result['status']='UNCONFIRMED_RETAINED_NO_SOURCE_WRITE'
        save(OUT/'terminal.json',result);save(CONTROL/'terminal.json',result)
    print(json.dumps(result,ensure_ascii=False))
    return failure is None and result['status']=='PASS_CLONE_SCHEMA_UPGRADE_ONLY_SOURCE_UPGRADE_NOT_AUTHORIZED'

if __name__=='__main__':raise SystemExit(0 if main() else 1)
