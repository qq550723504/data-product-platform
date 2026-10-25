"""Commit the completed packet locally for containment; never push or enable."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
BASE=HERE.parent
PACKET=BASE/'remaining-candidate22'
PREP=BASE/'remaining-candidate22-preparation01'
READ=BASE/'remaining-candidate22-readonly01'
P='a50b5a6307e79896578669ed53a3e12f08a0a815'
load=lambda path:json.loads(path.read_bytes())
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
def need(value,message):
    if not value:raise ValueError(message)
def git(*args):
    return subprocess.check_output(['git','-c','core.longpaths=true','-c','core.autocrlf=false',*args],cwd=ROOT)

def main():
    complete=load(READ/'complete.json');terminal=load(PREP/'non-delete-terminal.json')
    candidate=load(PACKET/'candidate.json');frozen=load(PACKET/'source-freeze.json')
    need(complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY' and complete['present_versions']==17
         and complete['native_delete']==0,'actual complete native read required')
    need(terminal['status']=='PASS_NONDELETE_SOURCE_RESTORE_PREPARATION'
         and terminal['groups_completed']==3 and terminal['native_delete']==0,'actual three-group native preparation required')
    need(candidate['Execution']=='0e5cb8f3-46a9-4339-88e4-7154d6ebe13f'
         and frozen['host']['source']==P,'source/execution binding differs')
    need(git('rev-parse','HEAD').decode().strip()==P,'source HEAD changed during preparation')
    need(not git('diff','--cached','--name-only').strip(),'unrelated staged changes must be reviewed first')
    need(not git('diff','--name-only',P,'--','apps/platform','examples/lifecycle-d1','migrations','.github/workflows/ci.yml').strip(),'host source changed')
    receipts=[row['path'] for row in load(HERE/'v29-checkout-raw-restoration02.json')['repairs']]
    for row in load(HERE/'v29-checkout-raw-restoration02.json')['repairs']:
        need(sha(ROOT/row['path'])==row['approved_original_sha256'],'restored raw receipt changed')
    members=[str(path.relative_to(ROOT)) for path in (PACKET,PREP,READ)]+receipts
    git('add','--',*members)
    git('diff','--cached','--check')
    result=subprocess.run(['git','-c','core.longpaths=true','commit','-qm','test(lifecycle): anchor candidate22 native nondelete packet'],cwd=ROOT,capture_output=True)
    for name in ('stdout','stderr'):
        with (HERE/('packet-commit01.'+name)).open('xb') as stream:stream.write(getattr(result,name))
    need(result.returncode==0,'local packet commit failed; preserve staged facts')
    head=git('rev-parse','HEAD').decode().strip()
    for name in ('candidate.json','bindings.json','confirmation-template.txt'):
        path=PACKET/name;need(git('show',head+':'+path.relative_to(ROOT).as_posix())==path.read_bytes(),'packet Git raw drift')
    for name in receipts:need(git('show',head+':'+name)==(ROOT/name).read_bytes(),'restored receipt Git raw drift')
    receipt={'status':'LOCAL_C22_PACKET_CONTAINMENT_ANCHOR_NOT_AUTHORIZED','head':head,'parent':P,
             'runtime_source':candidate['Source'],'candidate_sha256':sha(PACKET/'candidate.json'),
             'bindings_sha256':sha(PACKET/'bindings.json'),'template_sha256':sha(PACKET/'confirmation-template.txt'),
             'restored_v29_original_receipts_committed':len(receipts),'native_delete':0,'pushed':False,'authorization':False}
    with (HERE/'packet-commit01.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
    print(json.dumps(receipt))

if __name__=='__main__':main()
