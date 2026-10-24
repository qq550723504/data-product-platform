"""One conditional owner-authorized unlock; reuse GitHub's native rule API.

Only lockBranch=false is sent. Other protection fields are not sent, reset or
deleted. Unknown outcomes require read-only resolution, never mutation replay.
"""
import argparse
import copy
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
OLD_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-c21/data-product-platform')
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
REL='docs/acceptance/evidence/issue307-d1'
BASE=Path(__file__).parent/'authorized-maintenance01'
HEAD='f4f27c789f53b6c45910f55f83bee6cabfdee4f6'
RULE='BPR_kwDOUdQLBs4FE_7b'
BINDING='f27a617f278fcbaf64db29ddf536d5816d88acaba8a0d760280eb23bbe55d0de'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import execution_freeze as f
import retained_v31 as retained
import remaining_sequence as sequence


def before_gate(snapshot,binding):
    f.ready(snapshot,HEAD,require_frozen=True)
    approved=binding['branch_lock']['after']
    f.need(snapshot['rule']==approved['rule'] and snapshot['rule']['id']==RULE
           and snapshot['protection']==approved['protection'],'full approved native protection drift')
    return {'branchProtectionRuleId':RULE,'lockBranch':False}


def after_gate(before,after):
    f.ready(after,HEAD)
    expected_rule=copy.deepcopy(before['rule']);expected_rule['lockBranch']=False
    expected_protection=copy.deepcopy(before['protection']);expected_protection['lock_branch']['enabled']=False
    f.need(after['rule']==expected_rule and after['protection']==expected_protection,
           'unlock changed other protection fields or unlock not confirmed')


def closed_gate(out):
    spec=importlib.util.spec_from_file_location('original_c21_history_for_maintenance',OLD_ROOT/'examples/lifecycle-d1/prepare-six-candidate.py')
    old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
    count=0
    def call(args,data=None,output=None):
        nonlocal count
        args=list(map(str,args))
        if args[0]=='docker':
            f.need(args[1] in ('inspect','ps','cp') or args[1:3] in (['volume','inspect'],['network','inspect']),'maintenance attempted non-read Docker')
        else:f.need(args[0]=='git' and any(x in args for x in ('show','ls-files','rev-parse','cat-file')),'unknown history process')
        f.need(data is None and output is None,'unexpected history I/O')
        result=subprocess.run(args,cwd=OLD_ROOT,capture_output=True,timeout=40);count+=1
        f.save(out/('%04d-host-read.json'%count),dict(argv=args,exit=result.returncode,at=f.now(),
            stdout_sha256=f.digest(result.stdout),stderr_sha256=f.digest(result.stderr)))
        f.need(result.returncode==0,'host preservation read failed')
        return result.stdout
    old.call=call;old.p1.call=call;old.native.call=call
    prior=old.historical_gate()
    f.need(prior['status']=='PASS_RETAINED_907_CLOSED','original full retained chain incomplete')
    ids=set(prior['closed_cids'])
    c21=retained.verify(ROOT,old.inspect,old.closed,old.project_owner,old.docker,sequence,ids,PRIVATE,old.p1.hostpath)
    # The prior gate checks its own fixed states. Reobserve the complete known
    # union now as well; do not use yesterday's state projection as a fresh read.
    for n in range(0,len(ids),48):
        wanted=sorted(ids)[n:n+48];rows=json.loads(call(['docker','inspect',*wanted]))
        f.need(len(rows)==len({x['Id'] for x in rows})==len(wanted)
               and {x['Id'] for x in rows}==set(wanted) and all(old.closed(x) for x in rows),'known owner not freshly closed')
    result=dict(status='PASS_FRESH_996_CLOSED_OWNER_MAINTENANCE_GATE',at=f.now(),host_read_calls=count,
                retained=c21,source_start=0,native_delete=0)
    f.save(out/'result.json',result)
    return result


def run(output,*,apply=False):
    f.need(output.parent.resolve()==BASE.resolve() and re.fullmatch(r'operation[0-9]{2}',output.name)
           and not BASE.is_symlink() and not BASE.is_junction() and not output.exists(),
           'new local operation journal required')
    # A prior mutation intent, including an unknown outcome, is never replayed.
    f.need(not list(BASE.glob('operation*/mutation-intent.json')),'previous mutation attempt requires read-only resolution')
    output.mkdir()
    authority=f.load(BASE/'human-authority.json')
    f.need(authority['status']=='DIRECT_HUMAN_MAINTENANCE_AUTHORITY_RECEIVED'
           and authority['scope']=='CONDITIONAL_EXACT_LOCKBRANCH_FALSE_ONLY_THEN_RELOCK_AFTER_FINAL_CI'
           and authority['proposal_sha256']==f.sha(BASE.parent/'protection-owner-maintenance-proposal01.json')
           and authority['delete_authorization'] is False,'maintenance authority differs')
    old=PRIVATE/'remaining-candidate21'
    f.need(f.sha(old/'execution-binding.json')==BINDING
           and f.sha(old/'preparation-terminal.json')==retained.TERMINAL_SHA256,'old binding/terminal drift')
    binding=f.load(old/'execution-binding.json')
    journal=output/'github';journal.mkdir()
    client=f.GitHub(journal)
    before=client.snapshot();f.save(output/'before.json',before);before_gate(before,binding)
    host=output/'closed';host.mkdir();closure=closed_gate(host)
    # No CAS is available. Re-read after the potentially long host audit.
    fresh=client.snapshot();f.save(output/'fresh-before-mutation.json',fresh)
    body=before_gate(fresh,binding)
    if not apply:
        value=dict(status='PASS_FRESH_OWNER_UNLOCK_PRECONDITIONS_CHECK_ONLY',body=body,closure=closure,
                   branch_unlock=0,native_delete=0)
        f.save(output/'result.json',value);return value
    f.save(output/'mutation-intent.json',dict(at=f.now(),operation='updateBranchProtectionRule',input=body,
           expected_head=HEAD,expected_rule_id=RULE,other_fields='PRESERVE',atomic_CAS=False,automatic_retry=False))
    query='mutation($input:UpdateBranchProtectionRuleInput!){updateBranchProtectionRule(input:$input){branchProtectionRule{'+f.RULE_FIELDS+'}}}'
    response=client.graph(query,{'input':body});f.save(output/'mutation-response.json',response)
    after=client.snapshot();f.save(output/'after.json',after);after_gate(fresh,after)
    value=dict(status='PASS_OWNER_AUTHORIZED_EXACT_LOCKBRANCH_ONLY_UNLOCK',at=f.now(),head=HEAD,rule_id=RULE,
               native_rule_retained=True,other_protection_fields='FULL_EXACT_MATCH',CI='OLD_HEAD_ONLY_NOT_NEW_SOURCE_CI',
               next='PREPARE_NEW_SOURCE_CI_THEN_INDEPENDENT_RESOURCES_FINAL_CI_AND_RELOCK',
               PR332='OPEN_DRAFT_UNMERGED',Issue307='OPEN',native_delete=0,delete_authorization=False)
    f.save(output/'result.json',value)
    return value


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--output',required=True,type=Path)
    parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    output_preexisted=args.output.exists()
    try:
        result=run(args.output.resolve(),apply=args.apply)
        print(json.dumps({key:value for key,value in result.items() if key!='closure'}))
    except BaseException as error:
        if not output_preexisted and args.output.is_dir() and not (args.output/'failure.json').exists():
            f.save(args.output/'failure.json',dict(status='STOPPED_OWNER_MAINTENANCE_NOT_CONFIRMED',at=f.now(),
                error_type=type(error).__name__,message=str(error),automatic_mutation_retry=False,native_delete=0))
        raise
