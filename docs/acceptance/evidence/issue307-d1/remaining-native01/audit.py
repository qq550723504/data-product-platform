"""Offline integrity/semantic audit. No Docker, SQL, provider, or authority I/O."""
import hashlib
import json
from pathlib import Path
import subprocess

HERE=Path(__file__).resolve().parent
ROOT=HERE.parents[4]

def read(name):return json.loads((HERE/name).read_text(encoding='utf-8-sig'))
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def policy_sets(value):
    if isinstance(value,dict):return {k:policy_sets(v) for k,v in value.items()}
    if isinstance(value,list):return sorted([policy_sets(v) for v in value],key=canonical)
    return value

def audit():
    c=read('candidate06-preparation.json')
    assert c['current_delete_budget']==c['real_delete']==c['cleanup']==c['bypass']==c['hold_release']==0
    assert c['active_window'] is None and c['human_confirmation']=='NOT_REQUESTED_NOT_RECEIVED'
    assert c['authorization_status']=='BLOCKED_NATIVE_CASE_RUNNER_AND_NEW_EXECUTABLE_FREEZE'
    assert c['reference_operator_compatibility']=='OLD_V15_ONLY; NEW_REMAINING_PROFILE_UNSUPPORTED'
    assert len(c['prospective_targets'])==7 and len(c['survivors'])==9 and len(c['groups'])==3
    assert c['target_digest']==hashlib.sha256(canonical(c['prospective_targets'])).hexdigest()
    assert c['inventory_digest']==hashlib.sha256(canonical(c['prospective_targets']+c['survivors'])).hexdigest()
    assert c['source_freeze_sha256']==sha(HERE/'source-freeze.json')
    source=read('source-freeze.json');assert source['source_commit']==c['source_commit'] and len(source['files'])==768
    for path,hashes in source['files'].items():
        raw=(ROOT/path).read_bytes();assert hashlib.sha256(raw.replace(b'\r\n',b'\n')).hexdigest()==hashes['lf_sha256'],path
    calls=0
    for g in c['groups']:
        folder=Path(g['group']);manifest=read(folder/'manifest.json');checkpoint=read(folder/'checkpoint.json')
        assert sha(HERE/folder/'manifest.json')==g['manifest_sha256']
        assert sha(HERE/folder/'checkpoint.json')==g['checkpoint_sha256']
        assert checkpoint['status']=='PASS_FULL_CORE_AND_NATIVE_RESTORE' and checkpoint['core']['tables']==194
        assert read(folder/'source-before.json')==read(folder/'restored-core.json')
        assert checkpoint['installed_gateway']=='SEALED_DENY_ALL'
        assert checkpoint['active_gateway_sha256']==sha(HERE/folder/'gateway-active.Caddyfile')
        assert checkpoint['sealed_gateway_sha256']==sha(HERE/folder/'gateway-sealed.Caddyfile')
        verified=read(folder/checkpoint['native']['verification'])
        assert verified['status']=='PASS_NATIVE_READONLY' and verified['delete']==verified['mutations']==0
        assert verified['version_count']==len(manifest['objects']) and verified['disabled_users']==2
        assert verified['objects']==manifest['objects']
        expected=ROOT/'examples/lifecycle-d1/results'/manifest['run_id']/'delete-policy.json'
        assert sha(expected)==g['IAM']['delete_policy_sha256']
        native=HERE/folder/Path(checkpoint['native']['verification']).parent/'native-delete-policy.json'
        assert policy_sets(json.loads(expected.read_text()))==policy_sets(json.loads(native.read_text()))
        for name in ['lifecycle_disposition_approval','lifecycle_disposition_dispatch','lifecycle_provider_attempt','lifecycle_reader_pin']:
            assert manifest['facts'][name]==0
        assert manifest['facts']['lifecycle_storage_call']==3*len(manifest['objects'])+2
        calls+=manifest['facts']['lifecycle_storage_call']
        seals=read(folder/('seal-continuation03.json' if g['group']=='business-races' else 'seal.json'))
        assert all(x['status']=='exited' and x['pid']==0 and not any(x['ports'].values()) for x in seals['containers'])
        assert g['IAM']['inspection_status']==g['IAM']['delete_status']=='disabled'
    assert calls==54 and sum(len(g['plans']) for g in c['groups'])==8
    mixed=read('mixed/manifest.json')['objects'][:3]
    assert len({o['dataset_version_id'] for o in mixed})==len({o['sha256'] for o in mixed})==1
    assert mixed[2]['retention_mode']=='GOVERNANCE' and mixed[2]['retain_until']
    for name in ['business-races-attempt01/failure.json','business-races-attempt02/failure.json','business-races/native-restore-attempt01-failure.json','business-races/native-continuation01-failure.json','business-races/native-continuation02-failure.json']:
        assert read(name)['status'].startswith('FAIL')
    events=[json.loads(x) for x in (HERE/'prepare-unit-race.stdout.txt').read_text(encoding='utf-8-sig').splitlines()]
    assert not any(e['Action'] in {'fail','skip'} for e in events)
    assert events[-1]['Action']=='pass' and 'Test' not in events[-1]
    assert read('prepare-unit-race.json')['exit_code']==read('prepare-vet.json')['exit_code']==0
    index=read('evidence-index.json')
    for path,digest in index['files'].items():
        assert sha(HERE/path)==digest,path
    # Verify stored blobs too when tracked; captured CRLF must not normalize.
    tracked=set(subprocess.check_output(['git','ls-files','docs/acceptance/evidence/issue307-d1/remaining-native01'],cwd=ROOT).decode().splitlines())
    for path,digest in index['files'].items():
        relative=(HERE/path).relative_to(ROOT).as_posix()
        if relative in tracked:
            assert hashlib.sha256(subprocess.check_output(['git','show','HEAD:'+relative],cwd=ROOT)).hexdigest()==digest,relative
    return {'status':'PASS','groups':3,'versions':16,'plans':8,'tables_per_core':194,'typed_storage_calls':54,'retained_failure_receipts':5,'delete_budget':0,'native_race_acceptance':'NOT_RUN','send_authorization':'BLOCKED','indexed_files':len(index['files'])}

if __name__=='__main__':print(json.dumps(audit()))
