"""Revoke/close the unfrozen C20 allocation. Never start source or IAM."""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import sys

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
loader=importlib.util.spec_from_file_location('closed_unfrozen_runtime',ROOT/'examples/lifecycle-d1/six_candidate_runtime.py')
r=importlib.util.module_from_spec(loader);loader.loader.exec_module(r)
p=r.p
OUT=Path(__file__).parent/'ci-blocked-closure01'
PRIVATE=r.CONTROL/'ci-blocked-closure01'

def main():
    p.need(not OUT.exists() and not PRIVATE.exists(),'one-shot closure evidence exists')
    OUT.mkdir();PRIVATE.mkdir()
    a=r.load(r.CONTROL/'allocation.json')
    p.need(a['execution']==p.EXECUTION and p.EXECUTION=='5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6','fixed unfrozen allocation only')
    p.need(not (r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate20').exists()
           and not (r.PUBLIC/'non-delete-start.json').exists()
           and not (r.CONTROL/'authorized-execution01').exists(),'not the known pre-native unfrozen allocation')
    rows=[];failures=[];revoked=[]
    # Every known allocated owner is attempted even if another read fails.
    for cid in a['owners']:
        try:rows.append(p.p1.close_known(cid))
        except Exception as e:failures.append(dict(cid=cid,type=type(e).__name__))
    for g in a['groups']:
        folder=Path(g['private'])
        for path in sorted(folder.glob('*/permission.json')):
            cfg=r.load(path);p.need(cfg=={'Enabled':False},'unexpected unfrozen permission schema/state')
        profiles=list(folder.glob('case-*/remaining.json'))+list(folder.glob('rehearsal-*/remaining.json'))
        profiles+=list(folder.glob('maintenance-*/six-readonly.json'))+list(folder.glob('maintenance-*/six-provision.json'))
        for path in profiles:
            value=r.load(path);base=value['Base'] if 'Base' in value else value
            before=r.sha(path)
            if base['Enabled']:
                target=PRIVATE/path.relative_to(r.CONTROL);target.parent.mkdir(parents=True,exist_ok=True)
                r.write_raw(target,path.read_bytes());base['Enabled']=False;r.replace(path,value)
            revoked.append(dict(path=path.relative_to(r.CONTROL).as_posix(),before=before,after=r.sha(path),enabled=False))
        for role in ('Core','Native'):
            p.need(p.closed(p.inspect(g['source'][role])),'original source unexpectedly started')
        # All maintenance/case/worker/root owners were never started; only the
        # already completed budget initializer may have run.
        for cid in [*g['maintenance'].values(),*g['cases'].values(),*g['helpers'].values(),*g['root_owners'].values()]:
            x=p.inspect(cid);p.need(x['State']['StartedAt']==r.ZERO,'unexpected native/business/sender start')
    before=r.load(r.PUBLIC/'budget-initialization.json');budgets=[]
    for g in a['groups']:
        expected=next(x for x in before['groups'] if x['group']==g['group'])
        raw=r.docker('cp',g['helpers']['wire']+':/run/budget','-')
        p.need(hashlib.sha256(raw).hexdigest()==expected['archive_sha256'],'initialized unused budget drift')
        mounted=r.docker('ps','-aq','--no-trunc','--filter','volume='+g['volumes']['budget']).decode().split()
        p.need(sorted(mounted)==sorted(g['budget_expected_mounted_cids']),'new budget mount expansion')
        budgets.append(dict(group=g['group'],volume=g['volumes']['budget'],sha256=expected['archive_sha256'],empty=True,mounted_cids=sorted(mounted)))
    retained=p.historical_gate()
    p.need(retained['status']=='PASS_RETAINED_820_CLOSED','old retention failed')
    result=dict(status='FAILED_CI_BLOCKED_UNFROZEN_PREPARATION_CLOSED' if not failures and all(x['closed'] for x in rows) else 'UNCONFIRMED_CI_BLOCKED_PREPARATION',
                at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=p.EXECUTION,profile=p.PROFILE,
                host_source=a['host']['source'],runtime_source=a['source'],allocation_sha256=r.sha(r.PUBLIC/'allocation.json'),
                owners=rows,failures=failures,revoked_profiles=revoked,new_budgets=budgets,old_retained=retained,
                native_preparation='NOT_RUN',IAM='NOT_PROVISIONED_NO_SOURCE_START',source_started=False,source_business_commands=0,
                physical_provider_calls=0,native_delete=0,authorization='NOT_RECEIVED',schedule='NOT_CREATED',
                old_authorization_reused=False,old_pins_released=0,budgets_refunded=0,all_original_allocation_build_profiles_retained=True,
                retry_allowed=False,full_D1='NOT_ACCEPTED',PR332='DRAFT',Issue307='OPEN')
    r.save(OUT/'terminal.json',result);r.write_raw(PRIVATE/'terminal.json',(OUT/'terminal.json').read_bytes())
    print(json.dumps({k:result[k] for k in ['status','execution','source_started','native_delete','retry_allowed']}))

if __name__=='__main__':main()
