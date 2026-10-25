"""Finite local regression receipt; no retained environment lifecycle calls."""
import argparse,datetime,json,os,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
parser=argparse.ArgumentParser();parser.add_argument('--name',required=True);parser.add_argument('--cwd',type=Path);parser.add_argument('command',nargs=argparse.REMAINDER);args=parser.parse_args()
assert args.name.replace('-','').replace('_','').isalnum() and args.command
command=args.command[1:] if args.command[0]=='--' else args.command
folder=out/args.name;folder.mkdir(exist_ok=False)
env=os.environ.copy();env.update(PYTHONUTF8='1',PYTHONIOENCODING='utf-8',PYTHONDONTWRITEBYTECODE='1',CI='true',NEXT_TELEMETRY_DISABLED='1')
for key in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):env.pop(key,None)
started=datetime.datetime.now(datetime.timezone.utc).isoformat()
(folder/'start.json').write_text(json.dumps(dict(at=started,command=command,cwd=str(args.cwd or root),retained_source_start=0)),encoding='utf-8')
try:
 result=subprocess.run(command,cwd=args.cwd or root,env=env,capture_output=True,timeout=1200)
 for name,raw in [('stdout',result.stdout),('stderr',result.stderr)]:
  with (folder/name).open('xb') as stream:stream.write(raw)
 value=dict(started_at=started,finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),command=command,returncode=result.returncode,retained_source_start=0,provider_io=0,business_protection_write=0,delete=0)
except subprocess.TimeoutExpired as error:
 for name,raw in [('stdout',error.stdout or b''),('stderr',error.stderr or b'')]:
  with (folder/name).open('xb') as stream:stream.write(raw)
 value=dict(started_at=started,finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),error='REGRESSION_TIMEOUT_PRESERVE_KNOWN_TEST_OWNERS',retained_source_start=0)
 raise
except Exception as error:
 value=dict(started_at=started,finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),error=type(error).__name__,retained_source_start=0)
 raise
finally:
 with (folder/'result.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
 print(json.dumps(value),flush=True)
raise SystemExit(result.returncode)
