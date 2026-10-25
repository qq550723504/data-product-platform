"""Host-only observation of the expired frozen C21; never start or modify it."""
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
OLD_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-c21/data-product-platform')
REL=Path('docs/acceptance/evidence/issue307-d1')
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')
OUT=Path(__file__).parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit
import retained_v30 as retained
import remaining_sequence as sequence


def need(ok,message):
    if not ok:raise ValueError(message)


def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path):return json.loads(path.read_bytes())
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:
        stream.write(json.dumps(value,indent=2)+'\n')


def main():
    record=OUT/'retained-candidate21-anchor02.json'
    need(not record.exists(),'one-shot anchor already exists')
    spec=importlib.util.spec_from_file_location('original_c21_retained_helpers',OLD_ROOT/'examples/lifecycle-d1/prepare-six-candidate.py')
    p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
    calls=[]
    def docker(*args):
        need(args[0] in ('inspect','cp','ps') or args[:2] in (('volume','inspect'),('network','inspect')),'only host reads allowed')
        result=subprocess.run(['docker',*args],capture_output=True,check=True,timeout=40)
        calls.append(dict(argv=['docker',*args],exit=result.returncode,
                          stdout_sha256=hashlib.sha256(result.stdout).hexdigest(),stderr_sha256=hashlib.sha256(result.stderr).hexdigest()))
        return result.stdout
    terminal=OLD_ROOT/REL/'candidate21-expiry01/terminal.json'
    need(sha(terminal)==sha(PRIVATE/'preparation-terminal.json')=='5568708e62c4ad37f8765aba667697de547d83fe0e598d59d7d4341a84b4b188','terminal differs')
    need(not (PRIVATE/'authorized-execution01').exists(),'unexpected C21 authority')
    a=load(OLD_ROOT/REL/'remaining-candidate21-preparation01/allocation.json')
    candidate=load(OLD_ROOT/REL/'remaining-candidate21/candidate.json')
    descriptors=load(OLD_ROOT/REL/'remaining-candidate21-preparation01/owner-descriptors.json')
    baseline=load(OLD_ROOT/REL/'candidate21-planning01/final-cold-inventory01.json')
    ids=sorted({x['cid'] for x in baseline['owners']});need(len(ids)==996,'known owner membership differs')
    states={};allocated={};technical={}
    technical_ids={load(OLD_ROOT/REL/'remaining-candidate21-readonly01'/group/'allocated-owner.json')['cid'] for group in ('business-races','late-hold','mixed')}
    need(len(technical_ids)==3,'completed C21 readonly owner membership')
    for n in range(0,len(ids),48):
        for item in json.loads(docker('inspect',*ids[n:n+48])):
            need(p.closed(item),'known C21/retained owner active')
            states[item['Id']]={key:item['State'][key] for key in retained.STATE_FIELDS}
            if item['Id'] in descriptors:
                need(p.project_owner(item)==descriptors[item['Id']],'C21 owner descriptor differs')
                allocated[item['Id']]=dict(descriptor=descriptors[item['Id']],state=states[item['Id']])
            if item['Id'] in technical_ids:technical[item['Id']]=dict(descriptor=p.project_owner(item),state=states[item['Id']])
    need(len(allocated)==len(a['owners'])==86 and len(technical)==3,'C21 allocation/readonly membership differs')
    resources=[]
    for row in a['resources']:
        if 'volume' in row:
            v=json.loads(docker('volume','inspect',row['volume']))[0]
            mounted=docker('ps','-aq','--no-trunc','--filter','volume='+v['Name']).decode('utf-8','strict').split()
            resources.append(dict(kind='volume',value={key:v.get(key) for key in retained.VOLUME_FIELDS},mounted_cids=sorted(mounted)))
        else:
            v=json.loads(docker('network','inspect',row['network']))[0]
            need(v['Internal'] is True and not v.get('Containers'),'C21 restore network active')
            resources.append(dict(kind='network',value={key:v.get(key) for key in retained.NETWORK_FIELDS}))
    budgets=[]
    for group in a['groups']:
        raw=docker('cp',group['helpers']['wire']+':/run/budget','-')
        sequence.budget_archive(raw,candidate,group['group'],initial=True)
        budgets.append(dict(group=group['group'],volume=group['volumes']['budget'],
                            mounted_cids=group['budget_expected_mounted_cids'],sha256=hashlib.sha256(raw).hexdigest(),empty=True))
    names=['candidate21-planning01','candidate21-source-review01','candidate21-artifacts-review01',
           'candidate21-publication01','candidate21-crosscheck01','remaining-candidate21',
           'remaining-candidate21-preparation01','remaining-candidate21-readonly01','candidate21-expiry01']
    originals=[]
    for name in names:
        originals.extend(x for x in (OLD_ROOT/REL/name).rglob('*') if x.is_file() and '__pycache__' not in x.parts)
    originals.extend((OLD_ROOT/'docs/acceptance').glob('issue307-d1-candidate21*.md'))
    originals.extend((OLD_ROOT/'docs/acceptance').glob('issue307-d1-v31*.md'))
    members={};restorations=[]
    for source in sorted(set(originals)):
        source=audit.unlinked_absolute(source)
        need(source.stat().st_nlink==1,'original evidence aliased')
        relative=source.relative_to(OLD_ROOT.resolve()).as_posix();destination=ROOT/relative
        if destination.exists() and destination.read_bytes()!=source.read_bytes():
            current,issued=destination.read_bytes(),source.read_bytes()
            need(current.replace(b'\r\n',b'\n')==issued.replace(b'\r\n',b'\n'),'old evidence content differs: '+relative)
            archive=Path('C:/Users/Henry/.codex/fixture-private/candidate22-bootstrap01/checkout-raw-restoration01')/relative
            archive.parent.mkdir(parents=True,exist_ok=True)
            with archive.open('xb') as stream:stream.write(current)
            with destination.open('wb') as stream:stream.write(issued)
            with (ROOT/'.gitattributes').open('a',encoding='utf-8',newline='\n') as stream:stream.write('\n'+relative+' -text\n')
            restorations.append(dict(path=relative,before_sha256=hashlib.sha256(current).hexdigest(),issued_sha256=hashlib.sha256(issued).hexdigest(),LF_equivalent=True,original_new_checkout_preserved=str(archive)))
        if destination.exists():need(destination.read_bytes()==source.read_bytes(),'old evidence checkout bytes differ: '+relative)
        else:
            destination.parent.mkdir(parents=True,exist_ok=True)
            with destination.open('xb') as stream:stream.write(source.read_bytes())
        members[relative]=dict(sha256=sha(source),bytes=source.stat().st_size)
    save(OUT/'retained-c21-checkout-restoration01.json',dict(status='ISSUED_RECEIPT_BYTES_RESTORED_WITH_LF_EQUIVALENCE',restorations=restorations))
    save(OUT/'retained-candidate21-public-index02.json',dict(status='C21_ORIGINAL_PUBLIC_EVIDENCE_INDEX',count=len(members),files=members))
    result=dict(status='C21_RETAINED_EXPIRED_FROZEN_ANCHOR',at=dt.datetime.now(dt.timezone.utc).isoformat(),
                execution=a['execution'],profile=a['profile'],terminal_sha256=sha(terminal),
                private_files=retained.file_set(PRIVATE),owners=allocated,technical_owners=technical,known_states=states,
                resources=resources,budgets=budgets,authority_reusable=False,source_start=0,native_delete=0,host_read_calls=calls)
    save(record,result)
    print(json.dumps(dict(status=result['status'],anchor_sha256=sha(record),public_index_sha256=sha(OUT/'retained-candidate21-public-index02.json'),
                          public_members=len(members),private_members=len(result['private_files']),known_owners=len(states),new_resource_allocation=0,native_delete=0)))


if __name__=='__main__':main()
