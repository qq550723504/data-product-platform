"""Publish the two reviewed local commits by normal fast-forward only."""
import datetime as dt
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
OUT=Path(__file__).with_name('source-publication01');OUT.mkdir(exist_ok=False)
P='bee86b7961ee77eaa611f9ef8b8540362e8f6038'
R='81a083751c01a281d42df594f6e91012255081f1'
OLD='f4f27c789f53b6c45910f55f83bee6cabfdee4f6'
BRANCH='refs/heads/codex/issue307-post-v22-dispatch-closure'
RBRANCH='refs/heads/codex/issue307-candidate22-runtime'
count=0
def run(*args):
    global count
    count+=1
    command=['git','-c','core.longpaths=true','-c','core.autocrlf=false',*args]
    result=subprocess.run(command,cwd=ROOT,capture_output=True)
    for name in ('stdout','stderr'):
        (OUT/f'{count:02d}-{name}.txt').write_bytes(getattr(result,name))
    with (OUT/f'{count:02d}-request.json').open('x',encoding='utf-8') as stream:
        json.dump({'argv':command,'exit':result.returncode,'at':dt.datetime.now(dt.timezone.utc).isoformat()},stream,indent=2)
    if result.returncode:raise ValueError('normal Git publication failed; retain output, do not force or retry unknown push')
    return result.stdout.decode('utf-8').strip()

maintenance=json.loads((Path(__file__).parent/'authorized-maintenance01/operation03/result.json').read_bytes())
if maintenance['status']!='PASS_OWNER_AUTHORIZED_EXACT_LOCKBRANCH_ONLY_UNLOCK' or maintenance['head']!=OLD or maintenance['other_protection_fields']!='FULL_EXACT_MATCH':
    raise ValueError('actual exact-only unlock is not confirmed')
if run('rev-parse','HEAD')!=P:raise ValueError('P source differs')
if run('diff','--name-only','HEAD','--','apps/platform','examples/lifecycle-d1','migrations','.github/workflows/ci.yml'):
    raise ValueError('tracked host source drift')
if run('ls-files','--others','--exclude-standard','--','apps/platform','examples/lifecycle-d1','migrations','.github/workflows/ci.yml'):
    raise ValueError('untracked host source drift')
if run('ls-remote','origin',BRANCH)!=OLD+'\t'+BRANCH:raise ValueError('remote HEAD differs before normal FF')
if run('ls-remote','origin',RBRANCH):raise ValueError('new runtime branch already exists')
if run('merge-base',OLD,P)!=OLD:raise ValueError('P is not fast-forward')
run('push','origin',R+':'+RBRANCH)
run('push','origin',P+':'+BRANCH)
readback=run('ls-remote','origin',BRANCH,RBRANCH)
if set(readback.splitlines())!={P+'\t'+BRANCH,R+'\t'+RBRANCH}:raise ValueError('post-push remote refs differ')
receipt={'status':'PASS_TWO_NORMAL_FF_SOURCE_PUBLICATIONS_PENDING_NEW_HEAD_CI',
         'host_source':P,'runtime_source':R,'old_head':OLD,'native_delete':0,
         'delete_authorization':False,'review':'PENDING_INDEPENDENT_RESULT',
         'at':dt.datetime.now(dt.timezone.utc).isoformat()}
with (OUT/'result.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
print(json.dumps(receipt))
