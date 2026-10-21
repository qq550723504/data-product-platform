"""New GET/HEAD-only audit after the terminal v27 execution and trusted closure.

Reuses the complete native collector/checker; never reopens a deletion execution,
changes an old profile, enables an identity, or allocates a sender/budget.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess


def module(name, filename):
    spec=importlib.util.spec_from_file_location(name,Path(__file__).with_name(filename))
    value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value);return value


v=module('refresh_complete_six','verify-six-native.py')
c=module('refresh_closed_v27','close-v27-business-dispatches.py')
r,p=v.r,v.p
ROOT=r.ROOT
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/post-v27-seventeen01'
PRIVATE=c.PRIVATE/'post-v27-seventeen01'
EVIDENCE=ROOT/'docs/acceptance/evidence/issue307-d1'
BASELINES={
    'business-races':('post-v27-business-closure03/final-Core-fingerprint.json','ffdf75013a393da6a8444b28284fdfcc47b5a01a346f7fdf25a8c03138cd8a7f'),
    'late-hold':('remaining-candidate17/late-hold/core-readonly.json','7aafb25df2004970410fc80ebbe1206c49ebf2d3315d4294a95c7c903b10c01b'),
    'mixed':('remaining-candidate17/mixed/core-readonly.json','2c670718760a3a7306a9fdba4946e99d9fb024f7258235f4c7f6d880064afd55'),
}
ANCHORS={
    'remaining-candidate17/candidate.json':c.CANDIDATE,
    'remaining-candidate17/bindings.json':c.BINDINGS,
    'post-v27-business-closure03/terminal.json':'cdae9ec2ca7ee149bc73bad73456443ea82e988c647d9a775dd8d0403c396a7f',
    'host-path-repair01/evidence-index01.json':'5b08a4aceadce6c335c6e32a6871dccdc9a2c801933ad6c722d96e95adbffd79',
    'host-path-repair01/evidence-index02.json':'244926797024c5bcc236c0d895b1a061df32a0e6dccb0de21694a92c30d1ff11',
    'host-path-repair01/independent-final-followup02-review.json':'9e6c47648cf2728b4a9e8550f5efc3b5d5adc0cff2b192909aa85fa61c7f89f0',
}
need,load,sha,save=r.need,r.load,r.sha,r.save


def static_gate():
    for name,digest in {**ANCHORS,**dict(BASELINES.values())}.items():
        need(sha(EVIDENCE/name)==digest,'read-only historical anchor drift: '+name)
    terminal=load(EVIDENCE/'post-v27-business-closure03/terminal.json')
    need(terminal['status']=='PASS_V27_THREE_NOT_SENT_APPEND_ONLY_SEALED'
         and terminal['profile_disabled'] is True and terminal['retained_unchanged'] is True
         and terminal['native_delete']==terminal['pins_released']==terminal['budgets_refunded']==0,'trusted closure incomplete')
    report=load(EVIDENCE/'host-path-repair01/independent-final-followup02-review.json')
    need(report['status']=='PASS_BOUNDED_INDEPENDENT_HOST_PATH_AND_V27_CLOSURE_REVIEW'
         and report['remaining_blockers']==[],'bounded repair review incomplete')
    candidate,bindings=c.packet()
    need([g['Group'] for g in candidate['Groups']]==list(p.GROUPS),'source group order differs')
    for name,key in [('reader','ReaderBinarySHA256'),('readonly-check.exe',None)]:
        expected=candidate[key] if key else '223735f1154334496844e81a10abe92085f3c3c1506397dbbb9fa965348c0cf3'
        need(sha(r.CONTROL/name)==expected,'read-only runtime binary drift')
    need(load(c.CONTROL/'worker-private/closure.json')['Enabled'] is False,'closure observer enabled')
    return candidate,bindings


def snapshot():
    candidate,bindings=static_gate()
    physical=c.physical(candidate,bindings)
    retained=c.retained()
    need(retained==load(EVIDENCE/'post-v27-business-closure03/preparation.json')['retained'],'old budgets/profiles/backups/failure evidence drift')
    facts=load(EVIDENCE/'host-path-repair01/final-physical-and-Core-readback.json')
    ids=[x['cid'] for x in facts['owners']]
    need(len(ids)==len(set(ids))==620,'retained CID set differs')
    owners=c.inspect_many(ids)
    need(all(c.closed(x) for x in owners),'retained owner active')
    for g in candidate['Groups']:
        p.prior.closure.source_contract({'source':g})
        net=json.loads(c.docker('network','inspect',g['Network']))[0]
        need(net['Internal'] is True and not net.get('Containers'),'source network active or expanded')
    budget_mounts={}
    for volume in retained['budgets']:
        expected=sorted(x['Id'] for x in owners if any(m['Type']=='volume' and m['Name']==volume for m in x['Mounts']))
        actual=c.docker('ps','-aq','--no-trunc','--filter','volume='+volume).decode('utf-8','strict').split()
        need(len(actual)==len(set(actual)) and sorted(actual)==expected,'retained budget mount graph expanded')
        budget_mounts[volume]=expected
    descriptors={x['Id']:hashlib.sha256(json.dumps({k:x[k] for k in ('Image','Config','HostConfig','Mounts')},sort_keys=True).encode()).hexdigest() for x in owners}
    maintenance03={x.relative_to(c.CONTROL).as_posix():sha(x) for x in c.CONTROL.rglob('*') if x.is_file() and x.name!='host-key.private'}
    return dict(retained=retained,execution_owners=physical,descriptors=descriptors,budget_mounts=budget_mounts,maintenance03=maintenance03)


def code_inputs():
    return {x.name:sha(x) for x in sorted(Path(__file__).parent.glob('*.py'))}


def prepare():
    need(not PUBLIC.exists() and not PRIVATE.exists(),'one-shot read audit already allocated')
    state=snapshot()
    PUBLIC.mkdir();PRIVATE.mkdir()
    head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip()
    plan=dict(status='PREPARED_NEW_ZERO_DELETE_READ_AUDIT',source_head=head,code_inputs=code_inputs(),historical_execution=p.EXECUTION,
              deletion_authority='NONE',old_authorization_reused=False,baselines=BASELINES,preservation=state,
              activation_fields=['Enabled','NotBefore','ExpiresAt'],max_provider_deletes=0)
    save(PUBLIC/'plan.json',plan);save(PRIVATE/'plan.json',plan)
    print(json.dumps({'status':plan['status'],'plan_sha256':sha(PUBLIC/'plan.json'),'source_started':False}),flush=True)


def run(plan_sha):
    need(not (PUBLIC/'terminal.json').exists() and not (PRIVATE/'terminal.json').exists(),'read audit already terminal')
    need(sha(PUBLIC/'plan.json')==plan_sha==sha(PRIVATE/'plan.json'),'exact read audit plan differs')
    plan=load(PUBLIC/'plan.json')
    need(plan['status']=='PREPARED_NEW_ZERO_DELETE_READ_AUDIT' and plan['deletion_authority']=='NONE'
         and plan['old_authorization_reused'] is False and plan['max_provider_deletes']==0
         and plan['baselines']=={k:list(x) for k,x in BASELINES.items()},'read audit purpose/baseline expanded')
    need(plan['code_inputs']==code_inputs() and subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip()==plan['source_head'],'read audit source drift')
    candidate,bindings=static_gate()
    results=[];failure=None;started=r.now();deadline=started+dt.timedelta(minutes=50)
    save(PUBLIC/'readonly-lifetime.json',dict(started_at=started.isoformat(),expires_at=deadline.isoformat(),max_provider_deletes=0,deletion_window='NONE'))
    try:
        need(snapshot()==plan['preservation'],'pre-read preservation differs')
        for g,b in zip(candidate['Groups'],bindings):
            group=g['Group'];baseline=EVIDENCE/BASELINES[group][0]
            result=v.collect(g,b,v.PACKET,PRIVATE/group,PUBLIC/group,deadline,core_baseline=baseline)
            results.append(result)
        need([x['copies'] for x in results]==[7,6,4],'17 exact versions not all verified')
    except BaseException as error:
        failure=type(error).__name__+': '+str(error)
    finally:
        closure=[p.p1.close_known(cid) for g in candidate['Groups'] for cid in (g['Core'],g['Native'])]
        try:
            after=snapshot();save(PUBLIC/'preservation-after.json',after)
            need(after==plan['preservation'],'post-read retained facts drift')
            need(all(x['closed'] for x in closure),'source containment unconfirmed')
        except BaseException as error:failure=failure or type(error).__name__+': '+str(error)
        journals={}
        for g in candidate['Groups']:
            files=PUBLIC/g['Group']/'files'
            starts={x.stem[len('readonly-start-'):] for x in files.glob('readonly-start-*.json')}
            observations={x.stem[len('readonly-observation-'):] for x in files.glob('readonly-observation-*.json')}
            journals[g['Group']]=dict(starts=len(starts),observations=len(observations),unobserved_attempts=sorted(starts-observations))
        terminal=dict(status='FAILED_READ_AUDIT_RETAINED' if failure else 'PASS_CURRENT_SEVENTEEN_NATIVE_READONLY_SEALED',
                      at=r.now().isoformat(),error=failure,source_head=plan['source_head'],plan_sha256=plan_sha,
                      historical_execution=p.EXECUTION,deletion_authority='NONE',old_authorization_reused=False,
                      groups=results,completed_groups=[x['group'] for x in results],
                      not_completed_groups=[g['Group'] for g in candidate['Groups'] if g['Group'] not in [x['group'] for x in results]],
                      present_versions=sum(x['copies'] for x in results),
                      technical_requests=sum(x['starts'] for x in journals.values()),request_journals=journals,amount='NOT_ASSESSED',
                      closure=closure,native_delete=0,business_mutations=0,pins_released=0,budgets_refunded=0,D1='NOT_ACCEPTED')
        save(PUBLIC/'terminal.json',terminal);save(PRIVATE/'terminal.json',terminal)
    print(json.dumps({k:terminal[k] for k in ('status','error','present_versions','technical_requests','native_delete')}),flush=True)
    need(failure is None,failure or 'read audit failed')


def main():
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=('prepare','run'));parser.add_argument('--plan-sha256');args=parser.parse_args()
    if args.action=='prepare':prepare()
    else:
        need(args.plan_sha256,'exact zero-delete plan SHA required');run(args.plan_sha256)


if __name__=='__main__':main()
