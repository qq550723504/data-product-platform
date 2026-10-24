"""Actual native GitHub lock/write-rejection rehearsal; never a delete candidate."""
from pathlib import Path
import datetime as dt
import json
import subprocess
import sys
import uuid

ROOT=Path.cwd();HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import execution_freeze as f

out=HERE/'native-rehearsal01';out.mkdir(exist_ok=False)
branch='codex/issue307-freeze-rehearsal-'+uuid.uuid4().hex[:12]
client=f.GitHub(out,branch=branch)
head='17c5248a868376603ae9c3951123c545f2f7e56f'
f.save(out/'intent.json',dict(at=f.now(),scope='ISOLATED_GITHUB_BRANCH_ONLY_NOT_CANDIDATE',
       branch=branch,head=head,provider_io=0,native_delete=0,no_automatic_unlock=True))
prefix='repos/'+f.REPOSITORY
commit=client.request('GET',prefix+'/git/commits/'+head)
ref=client.request('POST',prefix+'/git/refs',{'ref':'refs/heads/'+branch,'sha':head})
f.need(ref['object']['sha']==head,'Created rehearsal ref differs')
# A genuine fast-forward target with exactly the same tree. Its update would
# be valid on the unprotected branch; the lock must be the rejecting rule.
marker=client.request('POST',prefix+'/git/commits',
       {'message':'#307 non-delete native branch-lock rehearsal; same tree',
        'tree':commit['tree']['sha'],'parents':[head]})
f.need(marker['tree']['sha']==commit['tree']['sha']
       and [p['sha'] for p in marker['parents']]==[head] and marker['sha']!=head,
       'Rehearsal update is not the exact same-tree fast-forward')
lock=f.acquire(client,head,out/'freeze')
request_rejected=False
try:
    client.request('PATCH',prefix+'/git/refs/heads/'+branch,
                   {'sha':marker['sha'],'force':False})
except ValueError:
    result=f.load(client.journal/(f'{client.sequence:03d}'+'-stdout.json'))
    message=json.dumps(result).lower()
    f.need('locked' in message and result.get('status') in ('403','409','422'),
           'Refusal was not an actual locked-branch rejection')
    request_rejected=True
f.need(request_rejected,'Native branch lock failed to reject a valid fast-forward')
after=client.snapshot()
f.ready(after,head,require_frozen=True)
f.need(after['rule']['id']==lock['rule_id']
       and after['protection']==lock['protection'],'Native locked ref/protection changed')
receipt=dict(status='PASS_NATIVE_GITHUB_LOCK_REJECTS_VALID_FAST_FORWARD',
             at=f.now(),scope='NONDELETE_REHEARSAL_NOT_EXECUTION_AUTHORITY',
             repository=f.REPOSITORY,branch=branch,locked_head=head,
             rule_id=lock['rule_id'],lock_flags={k:after['rule'][k] for k in f.FLAGS},
             marker_commit=marker['sha'],marker_same_tree=True,
             actual_update_attempts=1,actual_update_rejected=True,
             head_preserved=True,locked_branch_retained=True,
             candidate_created=False,provider_io=0,native_delete=0,
             source_start=0,budget_consumption=0,pin_release=0,
             original_PR_branch_changed=False)
f.save(out/'native-result.json',receipt)
print(json.dumps(receipt))

