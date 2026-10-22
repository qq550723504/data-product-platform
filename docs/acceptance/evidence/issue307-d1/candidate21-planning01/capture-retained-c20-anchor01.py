"""One-shot host-only C20 retention observation; no start/exec/provider verbs."""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as paths
spec=importlib.util.spec_from_file_location('retained_c20_projection',ROOT/'examples/lifecycle-d1/prepare-six-candidate.py')
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
OLD=PRIVATE/'remaining-candidate20'
OUT=Path(__file__).with_name('retained-candidate20-anchor01.json')
JOURNAL=PRIVATE/'candidate21-bootstrap01'/'retained-c20-journal01'


def digest(raw):return hashlib.sha256(raw).hexdigest()
def load(path):return json.loads(path.read_bytes())
def files(folder):
    values={}
    for path in sorted(folder.rglob('*')):
        if path.is_symlink() or path.is_junction():raise ValueError('linked old control')
        if not path.is_file():continue
        path=paths.unlinked_absolute(path)
        if path.stat().st_nlink!=1:raise ValueError('aliased old control')
        raw=path.read_bytes();values[path.relative_to(folder).as_posix()]={'sha256':digest(raw),'bytes':len(raw)}
    return values


def main():
    if OUT.exists():raise ValueError('one-shot retained anchor already exists')
    JOURNAL.mkdir();calls=[]
    def docker(*args):
        if not (args[0] in ('inspect','cp','ps') or args[:2] in (('volume','inspect'),('network','inspect'))):raise ValueError('non-read Docker verb')
        result=subprocess.run(['docker',*args],capture_output=True,check=True)
        # Raw inspect may include environment secrets. Only its SHA is public.
        calls.append({'argv':['docker',*args],'stdout_sha256':digest(result.stdout),'stderr_sha256':digest(result.stderr),'exit':result.returncode})
        return result.stdout
    terminal=BASE/'candidate20-planning01/ci-blocked-closure01/terminal.json'
    if digest(terminal.read_bytes())!='1aff9d0c516b4be3247e276e9b6a9ab2f9bb05af48e62f7f1dbe0cdcce28e9ae':raise ValueError('original terminal drift')
    if terminal.read_bytes()!=(OLD/'preparation-terminal.json').read_bytes():raise ValueError('private terminal drift')
    if (OLD/'authorized-execution01').exists():raise ValueError('unfrozen C20 unexpectedly authorized')
    a=load(BASE/'remaining-candidate20-preparation01/allocation.json');t=load(terminal)
    owners={}
    expected=load(BASE/'remaining-candidate20-preparation01/owner-descriptors.json')
    for n in range(0,len(a['owners']),48):
        for item in json.loads(docker('inspect',*a['owners'][n:n+48])):
            if not p.closed(item) or p.project_owner(item)!=expected[item['Id']]:raise ValueError('C20 owner not retained')
            state={k:item['State'][k] for k in ('Running','Pid','Status','ExitCode','StartedAt','FinishedAt','OOMKilled','Dead','Paused','Restarting')}
            owners[item['Id']]={'descriptor':p.project_owner(item),'state':state}
    if set(owners)!=set(a['owners']) or len(owners)!=86:raise ValueError('C20 owner membership')
    permissions={}
    for path in sorted(OLD.glob('*/*/permission.json')):
        if load(path)!={'Enabled':False}:raise ValueError('C20 permission not false')
        permissions[path.relative_to(OLD).as_posix()]=digest(path.read_bytes())
    if len(permissions)!=30:raise ValueError('C20 permission membership')
    resources=[]
    for resource in a['resources']:
        if 'volume' in resource:
            v=json.loads(docker('volume','inspect',resource['volume']))[0]
            mounted=docker('ps','-aq','--no-trunc','--filter','volume='+v['Name']).decode('utf-8','strict').split()
            resources.append({'kind':'volume','value':{k:v.get(k) for k in ('Name','Driver','Scope','Options','Labels','CreatedAt')},'mounted_cids':sorted(mounted)})
        else:
            v=json.loads(docker('network','inspect',resource['network']))[0]
            if v.get('Containers'):raise ValueError('C20 retained network has active owners')
            resources.append({'kind':'network','value':{k:v.get(k) for k in ('Id','Name','Driver','Scope','Internal','IPAM','Options','Labels','Containers')}})
    budgets=[]
    for row in t['new_budgets']:
        raw=docker('cp',row['mounted_cids'][0]+':/run/budget','-')
        if digest(raw)!=row['sha256']:raise ValueError('C20 original empty budget drift')
        budgets.append(row)
    clone=load(BASE/'candidate20-planning01/schema-compatibility01/allocation.json')
    item=json.loads(docker('inspect',clone['cid']))[0]
    if not p.closed(item) or item['HostConfig']['NetworkMode']!='none':raise ValueError('schema clone active')
    clone['descriptor']=p.project_owner(item)
    volume=json.loads(docker('volume','inspect',clone['volume']))[0]
    clone['volume_descriptor']={k:volume.get(k) for k in ('Name','Driver','Scope','Options','Labels','CreatedAt')}
    clone['volume_mounted_cids']=sorted(docker('ps','-aq','--no-trunc','--filter','volume='+clone['volume']).decode('utf-8','strict').split())
    result={'status':'C20_RETAINED_UNFROZEN_PREPARATION_ANCHOR','at':dt.datetime.now(dt.timezone.utc).isoformat(),
        'execution':a['execution'],'profile':a['profile'],'terminal_sha256':digest(terminal.read_bytes()),
        'owners':owners,'permissions':permissions,'resources':resources,'new_budgets':budgets,
        'private_files':files(OLD),'schema_clone':clone,'schema_private_files':files(PRIVATE/'candidate20-schema-compatibility01'),
        'original_failure_not_relabelled':True,'authority_reusable':False,'source_provider_actions':0,'native_delete':0,'read_calls':calls}
    with OUT.open('x',encoding='utf-8',newline='\n') as f:f.write(json.dumps(result,indent=2)+'\n')
    print(json.dumps({'status':result['status'],'sha256':digest(OUT.read_bytes()),'owners':len(owners),'private_files':len(result['private_files']),'resources':len(resources),'budgets':len(budgets),'host_read_calls':len(calls),'source_provider_actions':0}))

if __name__=='__main__':main()
