"""Offline-source/local-inspect freeze of the three prepared native groups.

No provider startup/RPC, SQL, IAM change, send or authorization mode. The old
sender/recovery are provenance references; this is not an executable candidate.
"""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import uuid

spec=importlib.util.spec_from_file_location('remaining_tools',Path(__file__).with_name('prepare-remaining-native.py'))
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

SEND_KEYS={'fixture/p1.csv':'P1','fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2','fixture/h2-a.csv':'H2','fixture/m1-a.csv':'M1','fixture/m1-b.csv':'M1'}

def canonical(value):
    return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()


def main():
    out=m.PUBLIC
    candidate=m.PRIVATE/'remaining-native01/candidate06-preparation'
    m.require(candidate.is_dir() and not (out/'candidate06-preparation.json').exists(),'One-shot freeze requires built private references')
    head=m.call(['git','rev-parse','HEAD']).decode().strip()
    m.require(not m.call(['git','status','--porcelain','--untracked-files=no']).strip(),'Tracked source must be committed')
    names=m.call(['git','ls-files','apps/platform','migrations','examples/lifecycle-d1']).decode().splitlines()
    sources={}
    for name in names:
        path=m.ROOT/name
        if path.suffix not in {'.go','.py','.ps1','.sql','.Caddyfile','.mod','.sum','.yaml','.yml'}:
            continue
        raw=path.read_bytes()
        sources[name]={'raw_sha256':hashlib.sha256(raw).hexdigest(),'lf_sha256':hashlib.sha256(raw.replace(b'\r\n',b'\n')).hexdigest()}
    m.write(out/'source-freeze.json',{'source_commit':head,'contract':'checkout raw SHA256 plus CRLF-to-LF-only SHA256; Git HEAD owns tracked source','files':sources})
    groups=[];targets=[];survivors=[]
    for group in m.GROUPS:
        folder=out/group
        state=m.load(m.PRIVATE/('remaining-native01/'+group+'.json'))
        private=Path(state['private']);resources=state['resources']
        manifest=m.load(folder/'manifest.json');checkpoint=m.load(folder/'checkpoint.json')
        m.require(manifest['run_id']==resources['run_id'] and checkpoint['status']=='PASS_FULL_CORE_AND_NATIVE_RESTORE','Native preparation scope incomplete')
        cfg=m.load(private/'config.json')
        m.require(not cfg['Enabled'] and manifest['iam']['delete_status']=='disabled' and manifest['iam']['inspection_status']=='disabled','Authority not disabled')
        m.require(checkpoint['sealed_gateway_sha256']==m.sha(private/'Caddyfile'),'Gateway seal changed')
        m.require(checkpoint['core']['dump_sha256']==m.sha(private/'checkpoint/core.dump') and checkpoint['core']['globals_sha256']==m.sha(private/'checkpoint/globals.sql') and checkpoint['native']['archive_sha256']==m.sha(private/'checkpoint/native.tar'),'Backup changed')
        finalseal=folder/('seal-continuation03.json' if group=='business-races' else 'seal.json')
        seals=m.load(finalseal)['containers']
        for seal in seals:
            current=m.inspect(seal['id'])
            m.require(current['Image']==seal['image'] and not current['State']['Running'] and current['State']['Pid']==0 and not any(current['NetworkSettings']['Ports'].values()),'Frozen topology changed')
        net=json.loads(m.docker('network','inspect',resources['network']))[0]
        m.require(net['Internal'] and net['Id']==manifest['environment']['network']['id'] and not net['Containers'],'Internal sealed network changed')
        m.require(all(manifest['facts'][name]==0 for name in ['lifecycle_disposition_approval','lifecycle_disposition_dispatch','lifecycle_provider_attempt','lifecycle_reader_pin']),'Execution already started')
        for obj in manifest['objects']:
            copy=dict(obj,group=group,workspace=manifest['workspace_id'],provider=manifest['provider_instance_id'],incarnation=manifest['incarnation_id'])
            case=SEND_KEYS.get(obj['key'])
            if case:
                plan=next(p for p in manifest['plans'] if p['case']==case)
                copy.update(case=case,plan=plan['id'],plan_hash=plan['scope_hash'],max_future_delete=1)
                m.require(not obj['legal_hold'] and obj['retention_mode']=='NONE','Protected future target')
                targets.append(copy)
            else:
                copy['max_future_delete']=0
                survivors.append(copy)
        verification=folder/checkpoint['native']['verification']
        m.require(m.load(verification)['status']=='PASS_NATIVE_READONLY','Native verification incomplete')
        groups.append({'group':group,'run_id':resources['run_id'],'project':resources['project'],'workspace':manifest['workspace_id'],'provider':manifest['provider_instance_id'],'incarnation':manifest['incarnation_id'],
                       'manifest_sha256':m.sha(folder/'manifest.json'),'checkpoint_sha256':m.sha(folder/'checkpoint.json'),'seal_sha256':m.sha(finalseal),'native_verification_sha256':m.sha(verification),
                       'preparation_source':manifest['environment']['source_head'],'prepare_binary_sha256':m.sha(private/'prepare'),'migrate_binary_sha256':m.sha(private/'migrate'),
                       'verifier_binary_sha256':checkpoint['native']['verifier_sha256'],'IAM':manifest['iam'],'active_gateway_sha256':checkpoint['active_gateway_sha256'],'sealed_gateway_sha256':checkpoint['sealed_gateway_sha256'],
                       'plans':manifest['plans'],'core':checkpoint['core'],'native_backup':checkpoint['native'],'network_id':net['Id'],'subnets':[x['Subnet'] for x in net['IPAM']['Config']],'installed_gateway':'SEALED_DENY_ALL'})
    m.require(len(targets)==7 and len(survivors)==9 and sum(len(g['plans']) for g in groups)==8,'Finite scope changed')
    mixed=m.load(out/'mixed/manifest.json')['objects'][:3]
    m.require(len({o['dataset_version_id'] for o in mixed})==len({o['sha256'] for o in mixed})==1 and mixed[2]['retention_mode']=='GOVERNANCE','Three-copy identity changed')
    identity={'candidate':'candidate06-preparation/v16','operator_execution_id':str(uuid.uuid4()),'kind':'PREPARATION_ONLY_NOT_A_BUSINESS_EXECUTION_OR_SEND_PERMIT'}
    m.write(candidate/'identity.json',identity)
    refs={name:m.sha(candidate/name) for name in ['reference-sender','reference-recovery']}
    for name in refs:
        (out/(name+'-modules.txt')).write_bytes(m.call(['go','version','-m',candidate/name]))
    result=dict(identity,source_commit=head,source_freeze_sha256=m.sha(out/'source-freeze.json'),source_file_count=len(sources),build='go1.26.5; linux/amd64; CGO_ENABLED=0; references: -trimpath -buildvcs=false, external SHA binding',
                reference_binaries_sha256=refs,reference_operator_compatibility='OLD_V15_ONLY; NEW_REMAINING_PROFILE_UNSUPPORTED',groups=groups,prospective_targets=targets,survivors=survivors,
                target_digest=hashlib.sha256(canonical(targets)).hexdigest(),inventory_digest=hashlib.sha256(canonical(targets+survivors)).hexdigest(),
                human_confirmation='NOT_REQUESTED_NOT_RECEIVED',current_delete_budget=0,platform_approvals=0,active_window=None,window_max_future_minutes=40,future_max_total=7,max_future_per_target=1,
                authorization_status='BLOCKED_NATIVE_CASE_RUNNER_AND_NEW_EXECUTABLE_FREEZE',reader_producer_identities='NOT_PROVISIONED',native_races='NOT_RUN',full_D1='NOT_ACCEPTED',
                real_delete=0,hold_release=0,bypass=0,cleanup=0,PR_323='DRAFT',issue_307='OPEN',old_authorizations='NOT_REUSED',
                required_before_new_confirmation=['actual Asynq/NATIVE and authenticated DirectData case preparation','restricted reader/producer/wire-pause/witness/recovery ordering operator','new seven-target sender and no-send/unknown/replay regressions','independent review of the complete executable candidate','fresh readonly preconditions and complete new source/binary/Core/IAM/gateway/execution/fixed UTC window bindings'])
    m.write(out/'candidate06-preparation.json',result)
    print(json.dumps({'candidate':identity,'source':head,'source_files':len(sources),'groups':len(groups),'versions':16,'plans':8,'current_delete_budget':0}))


if __name__=='__main__':main()
