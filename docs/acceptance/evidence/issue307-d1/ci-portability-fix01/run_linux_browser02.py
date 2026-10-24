"""One isolated mock-browser regression container, never a retained source."""
import json,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
private=Path.home()/'.codex/fixture-private/ci-portability-fix01/linux-browser-full02';private.mkdir(exist_ok=False)
folder=out/'linux-browser-container02';folder.mkdir(exist_ok=False)
def run(args):
 result=subprocess.run(args,capture_output=True,timeout=60)
 if result.returncode:raise RuntimeError(result.stderr.decode('utf-8','strict'))
 return result.stdout
image=json.loads(run(['docker','image','inspect','dpp307-ci-browser:portability02']))[0]['Id']
cid=run(['docker','create','--name','dpp307-ci-browser-portability02','--network','none','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges','--shm-size','1g','--mount','type=bind,source='+str(private)+',target=/out',image]).decode().strip()
before=json.loads(run(['docker','inspect',cid]))[0]
with (folder/'before.json').open('x',encoding='utf-8') as stream:json.dump(before,stream,indent=2)
command=[sys.executable,'-B',str(out/'capture_command01.py'),'--name','linux-browser-full02','--','docker','start','-a',cid]
result=subprocess.run(command,cwd=root,timeout=1250)
after=json.loads(run(['docker','inspect',cid]))[0]
with (folder/'after.json').open('x',encoding='utf-8') as stream:json.dump(after,stream,indent=2)
assert not after['State']['Running'] and after['State']['Pid']==0 and after['State']['ExitCode']==0 and result.returncode==0
with (folder/'result.json').open('x',encoding='utf-8') as stream:json.dump(dict(status='PASS_ISOLATED_LINUX_BROWSER_FULL_COLLECTION',cid=cid,image=image,network='none',ports=[],retained_mounts=[],runtime_uid=10001,exit_code=after['State']['ExitCode'],retained_source_start=0,business_provider_io=0),stream,indent=2)
print(json.dumps(dict(cid=cid,status='PASS_ISOLATED_LINUX_BROWSER_FULL_COLLECTION')))
