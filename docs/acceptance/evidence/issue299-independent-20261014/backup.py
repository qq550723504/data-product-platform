"""Native PostgreSQL backup/restore oracle for this new synthetic run only."""
import hashlib, json, secrets, subprocess, sys, time
from pathlib import Path

root=Path(__file__).resolve().parent
repo=root.parents[1]
runtime=repo/'.data/independent299-runtime'
own=json.loads((runtime/'ownership.json').read_text())
private=Path(own['private'])
phase=sys.argv[1]
backup=private/('backup-'+phase)
backup.mkdir(exist_ok=False)
suffix=own['identity'].removeprefix('dpp-pgi-')
source=own['identity']+'-source'
core='dpp-pgi-b3-core-'+suffix
restore='dpp-ind299-restore-'+phase+'-'+suffix
volume=restore+'-data'
image='postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'

def run(args, data=None):
    r=subprocess.run(args,input=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    if r.returncode:
        (backup/'failure.log').write_bytes(r.stdout+r.stderr)
        raise RuntimeError('native backup/restore command failed, see private failure.log')
    return r.stdout

def sql(container,db,q):
    return run(['docker','exec',container,'psql','-U','postgres','-d',db,'-At','-v','ON_ERROR_STOP=1','-c',q])

def fingerprint(container,db):
    tables=json.loads(sql(container,db,"SELECT coalesce(json_agg(format('%I.%I',schemaname,tablename) ORDER BY schemaname,tablename),'[]') FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')"))
    result={}
    for table in tables:
        rows=sql(container,db,f'SELECT row_to_json(t)::text FROM {table} t ORDER BY row_to_json(t)::text').splitlines()
        result[table]={'rows':len(rows),'sha256':hashlib.sha256(b'\n'.join(rows)).hexdigest()}
    return result

report={'phase':phase,'runId':own['run_id'],'status':'RUNNING','databases':{}}
for container,db in [(core,'dpp_ingestion_core'),(source,'dpp_ingestion_source')]:
    dump=run(['docker','exec',container,'pg_dump','-U','postgres','-d',db,'-Fc'])
    (backup/(db+'.dump')).write_bytes(dump)
    globals=run(['docker','exec',container,'pg_dumpall','-U','postgres','--globals-only'])
    (backup/(db+'-globals.sql')).write_bytes(globals)
    report['databases'][db]={'dumpSha256':hashlib.sha256(dump).hexdigest(),'before':fingerprint(container,db)}
password=backup/'restore-password'
password.write_text(secrets.token_hex(32))
run(['docker','volume','create','--label','dpp.ind299.run='+own['run_id'],volume])
run(['docker','create','--name',restore,'--label','dpp.ind299.run='+own['run_id'],'--network','dpp-pgi-b3-storage-private-'+suffix,
    '--mount',f'type=volume,src={volume},dst=/var/lib/postgresql/data',
    '--mount',f'type=bind,src={password},dst=/run/credentials/password,readonly',
    '-e','POSTGRES_PASSWORD_FILE=/run/credentials/password',image])
metadata=json.loads(run(['docker','inspect',restore]))[0]
assert metadata['State']['Status']=='created'
mount=[m for m in metadata['Mounts'] if m['Destination']=='/var/lib/postgresql/data']
assert len(mount)==1 and mount[0]['Type']=='volume' and mount[0]['Name']==volume and mount[0]['RW']
assert not metadata['HostConfig'].get('Tmpfs')
run(['docker','start',restore])
for _ in range(60):
    r=subprocess.run(['docker','exec',restore,'pg_isready','-U','postgres'],capture_output=True)
    if r.returncode==0:break
    time.sleep(.5)
else:raise RuntimeError('restore PostgreSQL unavailable')
# Source-specific roles are restored verbatim; the new cluster already owns postgres.
globals=(backup/'dpp_ingestion_source-globals.sql').read_text()
globals='\n'.join(line for line in globals.splitlines() if not line.startswith('CREATE ROLE postgres;') and not line.startswith('ALTER ROLE postgres '))
run(['docker','exec','-i',restore,'psql','-U','postgres','-v','ON_ERROR_STOP=1'],globals.encode())
for db,entry in report['databases'].items():
    run(['docker','exec','-i',restore,'pg_restore','-U','postgres','--create','-d','postgres','--exit-on-error'],(backup/(db+'.dump')).read_bytes())
    entry['restored']=fingerprint(restore,db)
    assert entry['before']==entry['restored'],db+' restore fingerprint mismatch'
report.update(status='PASS',restoreContainer=restore,restoreVolume=volume,backupPrivate=str(backup),oldIncident='FAIL / NOT_RESTORED')
(root/('backup-'+phase+'.json')).write_text(json.dumps(report,indent=2),encoding='utf-8')
print(json.dumps({'status':'PASS','phase':phase,'restoreContainer':restore,'tables':{k:len(v['before']) for k,v in report['databases'].items()},'rows':{k:sum(x['rows'] for x in v['before'].values()) for k,v in report['databases'].items()}}))
