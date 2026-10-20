import importlib,json,hashlib,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent;BASE=ROOT/'docs/acceptance/evidence/issue307-d1';PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate17')
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'));import remaining_six_window as windows
load=lambda p:json.loads(p.read_bytes());sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
packet=BASE/'remaining-candidate17';prep=BASE/'remaining-candidate17-preparation01';c=load(packet/'candidate.json');bindings=load(packet/'bindings.json');allocation=load(prep/'allocation.json');descriptors=load(prep/'owner-descriptors.json')
validated=windows.validate_packet(packet,PRIVATE);assert validated['cases']==validated['recoveries']==6
common=dict(Enabled=False,Execution=c['Execution'],CandidateSHA256=sha(packet/'candidate.json'),ConfirmationSHA256=sha(packet/'confirmation-template.txt'),NotBefore=c['NotBefore'],ExpiresAt=c['NotAfter'],ConfirmedAt='0001-01-01T00:00:00Z',MaxPerTarget=1,MaxTotal=6)
counts={'permissions_false':0,'operators':0,'root':0,'cases':0,'recoveries':0,'maintenance_disabled':0,'trial_disabled':0}
for g,b in zip(c['Groups'],bindings):
    folder=PRIVATE/g['Group']
    for stage,cid in g['Helpers'].items():
        target=folder/stage;cfg=load(target/'operator.json');permission=load(target/'permission.json')
        assert permission==dict(common,Profile=c['Profile'],BinarySHA256=c['SenderBinarySHA256']) and not (target/'human-confirmation.txt').exists()
        assert sha(target/'operator.json')==g['ConfigSHA256'][stage] and not any(k in cfg for k in ('RootAccess','RootSecret'))
        assert (target/'candidate.json').read_bytes()==(packet/'candidate.json').read_bytes() and (target/'confirmation-template.txt').read_bytes()==(packet/'confirmation-template.txt').read_bytes()
        if stage=='wire':assert not cfg['DeleteSecret'] and cfg['WirePrivateKey']
        else:assert not cfg['WirePrivateKey']
        if stage.startswith('recover-'):
            assert not cfg['DeleteSecret'];rcfg=load(target/'recovery.json');assert rcfg['ValidUntil']=='2026-10-21T08:40:00Z' and rcfg['Enabled'] is True
            counts['recoveries']+=1
        counts['permissions_false']+=1;counts['operators']+=1
    for action,owner in b['root_owners'].items():
        target=folder/('root-'+action);assert load(target/'permission.json')==common and not (target/'human-confirmation.txt').exists()
        assert sha(target/'root.json')==owner['root_config_sha256'];counts['permissions_false']+=1;counts['root']+=1
    for stage,cid in g['CaseHelpers'].items():
        target=folder/('case-'+stage);cfg=load(target/'remaining.json');assert sha(target/'remaining.json')==g['CaseConfigSHA256'][stage]
        assert not any(cfg['Base'].get(k) for k in ('RootAccess','RootSecret','ProducerAccess','ProducerSecret'))
        assert cfg['Base']['ManifestSHA256']==b['manifest_sha256'] and sha(target/'manifest.json')==b['manifest_sha256'];counts['cases']+=1
    for p in folder.glob('maintenance-*/*.json'):
        if p.name in ('six-provision.json','six-readonly.json'):
            v=load(p);assert v['Enabled'] is False and v['MaxProviderDeletes']==0;counts['maintenance_disabled']+=1
    for p in folder.glob('restore-inspect-*/*.json'):
        if p.name=='restore-inspector.json':assert load(p)['Enabled'] is False;counts['maintenance_disabled']+=1
    for p in folder.glob('rehearsal-*/*.json'):
        if p.name=='remaining.json':assert load(p)['Base']['Enabled'] is False;counts['trial_disabled']+=1
assert counts['maintenance_disabled']==15 and counts['trial_disabled']==4
assert counts['permissions_false']==30 and counts['root']==15 and counts['operators']==15 and counts['cases']==counts['recoveries']==6
key=(PRIVATE/'topology-key.private').resolve();assert key.is_file()
for cid,descriptor in descriptors.items():
    for mount in descriptor['mounts']:
        assert mount['Destination'] not in ('/var/run/docker.sock','/run/docker.sock')
        if mount['Type']!='bind':continue
        source=Path(mount['Source'])
        if not source.is_absolute():continue
        resolved=source.resolve()
        assert resolved!=key and not (resolved.is_dir() and key.is_relative_to(resolved)),(cid,mount['Destination'])
assert len(descriptors)==len(allocation['owners'])==86
budget=load(prep/'budget-initialization.json');assert budget['all_complete'] is True
assert len(budget['groups'])==3 and budget['failure'] is None and all(x['closed'] for x in budget['closure'])
for row in budget['groups']:
    assert row['status']=='PASS_EXACT_SIX_BUDGET' and row['initial_empty'] is True and row['uid']==row['gid']==10001 and row['files']==[]
# A source/head claim is only a binding, not identity enable authority.
policy=load(ROOT/'apps/platform/internal/lifecycle/pilot/remaining_policy_heads.json')
for version,head in c['Groups'][0]['PolicyHeads'].items():assert version in json.dumps(policy) and head in json.dumps(policy)
assert len(c['Groups'][0]['PolicyHeads'])==5
absence=load(BASE/'candidate17-planning01/authority-schedule-absence01.json');assert not any(path.name=='human-authorization.json' for path in (BASE/'delete-confirmed-v27-execution01').glob('*'))
report={'status':'PASS_FROZEN_PRIVATE_FILE_SCOPE_AND_PERMISSION_BINDINGS','validated_packet':validated,'counts':counts,'build_owner_descriptors':86,'host_private_key_not_bound_into_any_allocated_owner':True,'docker_socket_mounts':0,'budget_initialization_receipt_sha256':sha(prep/'budget-initialization.json'),'authority_schedule_absence_receipt_sha256':sha(BASE/'candidate17-planning01/authority-schedule-absence01.json'),'five_actual_policy_heads_bound':True,'frozen_case_and_recovery_static_enabled_values':'Exact immutable preparation bytes; physical owners stopped and permissionsfalse. Not activated by this review.','private_profile_values_or_secrets_output':False,'readiness_scope':'File-only; physical and budget live state taken from original fixed host gate and final-cold parent receipt, not operated by reviewer.','reviewer_Docker_Source_provider_enable_delete':0}
with (OUT/'private-file-safety-audit02.json').open('xb') as f:f.write((json.dumps(report,indent=2)+'\n').encode())
print(json.dumps({'status':report['status'],'counts':counts}))
