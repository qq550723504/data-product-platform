"""Native three-database restore of this run's stopped OpenMetadata only."""
import hashlib,json,secrets,subprocess,time,sys,re
from pathlib import Path
root=Path(__file__).resolve().parent;repo=root.parents[1]
state=json.loads((repo/'.data/independent299-runtime/runtime.json').read_text())
phase=sys.argv[1] if len(sys.argv)>1 else 'final'
assert re.fullmatch(r'final[23]?',phase)
backup=Path(state['private'])/('backup-metadata-'+phase);backup.mkdir(exist_ok=False)
source='dpp-pgi09-ind299-20261014-postgresql'
restore='dpp-ind299-restored-ompg-47203cb12b93'+('' if phase=='final' else '-'+phase);volume=restore+'-data'
def run(args,data=None):
 r=subprocess.run(args,input=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if r.returncode:
  (backup/'failure.log').write_bytes(r.stdout+r.stderr)
  raise RuntimeError('native metadata backup failed; see private log')
 return r.stdout
def sql(container,db,q):return run(['docker','exec',container,'psql','-U','postgres','-d',db,'-At','-v','ON_ERROR_STOP=1','-c',q])
def fingerprint(container,db):
 tables=json.loads(sql(container,db,"SELECT coalesce(json_agg(format('%I.%I',schemaname,tablename) ORDER BY schemaname,tablename),'[]') FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')"))
 result={}
 for table in tables:
  rows=sql(container,db,f'SELECT row_to_json(t)::text FROM {table} t ORDER BY row_to_json(t)::text').splitlines()
  result[table]={'rows':len(rows),'sha256':hashlib.sha256(b'\n'.join(rows)).hexdigest()}
 return result
sourceInfo=json.loads(run(['docker','inspect',source]))[0]
server=json.loads(run(['docker','inspect','dpp-pgi09-ind299-20261014-openmetadata-server']))[0]
assert server['State']['Status']=='exited','metadata server must be quiescent'
report={'runId':state['run_id'],'status':'RUNNING','databases':{},'image':sourceInfo['Image']}
dbs=sql(source,'postgres','SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY datname').decode().splitlines()
for db in dbs:
 dump=run(['docker','exec',source,'pg_dump','-U','postgres','-d',db,'-Fc'])
 (backup/(db+'.dump')).write_bytes(dump)
 report['databases'][db]={'dumpSha256':hashlib.sha256(dump).hexdigest(),'before':fingerprint(source,db)}
globals=run(['docker','exec',source,'pg_dumpall','-U','postgres','--globals-only'])
(backup/'globals.sql').write_bytes(globals)
password=backup/'restore-password';password.write_text(secrets.token_hex(32))
run(['docker','volume','create','--label','dpp.ind299.run='+state['run_id'],volume])
run(['docker','create','--name',restore,'--label','dpp.ind299.run='+state['run_id'],'--network','dpp-pgi09-ind299-20261014_app_net',
 '--mount',f'type=volume,src={volume},dst=/var/lib/postgresql/data',
 '--mount',f'type=bind,src={password},dst=/run/credentials/password,readonly',
 '-e','POSTGRES_PASSWORD_FILE=/run/credentials/password',sourceInfo['Image']])
run(['docker','start',restore])
for _ in range(60):
 r=subprocess.run(['docker','exec',restore,'pg_isready','-h','127.0.0.1','-U','postgres'],capture_output=True)
 if r.returncode==0:break
 time.sleep(.5)
else:raise RuntimeError('restored metadata PostgreSQL unavailable')
existing=set(sql(restore,'postgres','SELECT rolname FROM pg_roles').decode().splitlines())
lines=globals.decode().splitlines()
lines=[line for line in lines if not any(line==f'CREATE ROLE {role};' for role in existing)]
run(['docker','exec','-i',restore,'psql','-U','postgres','-v','ON_ERROR_STOP=1'],'\n'.join(lines).encode())
current=set(sql(restore,'postgres','SELECT datname FROM pg_database').decode().splitlines())
for db,entry in report['databases'].items():
 if db not in current:sql(restore,'postgres','CREATE DATABASE "'+db+'"')
 run(['docker','exec','-i',restore,'pg_restore','-U','postgres','-d',db,'--exit-on-error'],(backup/(db+'.dump')).read_bytes())
 entry['restored']=fingerprint(restore,db)
 assert entry['before']==entry['restored'],db+' metadata restore mismatch'
report.update(status='PASS',restoreContainer=restore,restoreVolume=volume,backupPrivate=str(backup),providerDelete=0,cleanup=0)
(root/('metadata-backup-'+phase+'.json')).write_text(json.dumps(report,indent=2))
print(json.dumps({'status':'PASS','databases':{db:len(entry['before']) for db,entry in report['databases'].items()},'restoreContainer':restore}))
