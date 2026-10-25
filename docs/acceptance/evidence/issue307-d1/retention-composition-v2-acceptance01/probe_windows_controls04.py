"""Actual Windows UNC r+b/fsync against the new isolated synthetic volume."""
import hashlib,json,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import retention_composition as composition
import retention_staging as staging
out=Path(__file__).parent
probe=json.loads((out/'linux-probe01.json').read_bytes())
if probe['status']!='PASS_ISOLATED_LINUX_CUSTODY_FSYNC_WAIT_TESTS':raise ValueError('prior isolated probe not PASS')
view=probe['views']['private']
if view['metadata']['Name']!='d1-retention-probe-private-4598c8c87879':raise ValueError('only this new synthetic volume is approved for the probe')
path=Path(view['windows'])/'control.json'
old=path.read_bytes()
if old!=b'{"test_only":true}':raise ValueError('synthetic control drift')
archive=Path.home()/'.codex/fixture-private/retention-composition-v2-acceptance01/host-control-probe'
archive.mkdir(exist_ok=False)
failure=None
try:
 composition.replace_control(path,b'{"test_only":true,"stage":"durable-host-control"}',archive/'original-control.json')
 result=staging.validate_directory(view['metadata'],view['windows'],files={'control.json'})
 if not composition.present(path) or composition.present(archive/'not-created'):raise ValueError('actual present/absence contract failed')
except Exception as error:
 failure=dict(type=type(error).__name__,message=str(error));raise
finally:
 value=dict(status='PASS_ACTUAL_WINDOWS_UNC_FSYNC_AND_LINUX_CUSTODY' if failure is None else 'FAILED_NEW_SYNTHETIC_HOST_CONTROL_PRESERVED',failure=failure,
  volume=view['metadata']['Name'],original_sha256=hashlib.sha256(old).hexdigest(),final_sha256=hashlib.sha256(path.read_bytes()).hexdigest(),original_preserved=(archive/'original-control.json').read_bytes()==old,
  actual_linux_snapshot=locals().get('result'),retained_source_start=0,provider_io=0,protection_write=0,delete=0,cleanup=0)
 with (out/'windows-control-probe04.json').open('x',encoding='utf-8') as stream:json.dump(value,stream,indent=2)
 print(json.dumps({k:v for k,v in value.items() if k!='actual_linux_snapshot'}))
