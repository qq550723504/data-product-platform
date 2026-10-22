"""Independent read-only packet, native-receipt and document binding audit."""
import datetime as dt
import hashlib
import json
import re
import subprocess
import tomllib
from pathlib import Path

ROOT = Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
OUT = Path(__file__).parent
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
PACKET = BASE / 'remaining-candidate11'
NATIVE = BASE / 'remaining-preflight-native04'
REVIEW = BASE / 'remaining-window-review05'
ANCHOR = 'c00d4a8f85aaaaa71b9c19c76d042dee6aad1802'
EXECUTION = '602acafe-065a-4385-b739-5776408f5448'

def need(ok, message):
    if not ok:
        raise ValueError(message)

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def load(path):
    return json.loads(path.read_bytes())

def audit():
    c = load(PACKET/'candidate.json')
    b = load(PACKET/'bindings.json')
    idx = load(PACKET/'evidence-index.json')
    entries = {**idx['files'], **idx['references']}
    need(len(entries) == 1122, 'raw evidence count')
    # Read exact Git blob bytes through one batch operation, with no checkout or I/O
    # to Docker, native providers, private profiles or permission files.
    names = sorted(entries)
    query = ''.join(ANCHOR+':'+n+'\n' for n in names).encode()
    raw = subprocess.run(['git','cat-file','--batch'], cwd=ROOT, input=query,
                         stdout=subprocess.PIPE, check=True).stdout
    offset = 0
    for name in names:
        end = raw.index(b'\n', offset)
        header = raw[offset:end].decode('ascii').split()
        need(len(header) == 3 and header[1] == 'blob', 'missing Git blob '+name)
        size = int(header[2]); offset = end + 1
        body = raw[offset:offset+size]; offset += size + 1
        need(hashlib.sha256(body).hexdigest() == entries[name], 'Git raw mismatch '+name)
        need((ROOT/name).read_bytes() == body, 'working/Git raw mismatch '+name)
    need(offset == len(raw), 'unparsed Git data')

    doc_path = ROOT/'docs/acceptance/issue307-d1-candidate11-confirmation.md'
    run_path = ROOT/'docs/acceptance/issue307-d1-v21-window-runbook.md'
    doc = doc_path.read_bytes()
    template = (PACKET/'confirmation-template.txt').read_bytes()
    embedded = re.findall(rb'```text\n(.*?)```', doc, re.S)
    need(embedded == [template], 'machine template is not embedded byte-for-byte')
    text = template.decode('utf-8')
    doc_text = doc.decode('utf-8')
    run = run_path.read_text(encoding='utf-8')
    need(c['Execution'] == EXECUTION and c['Source'] == '3d62dacb36b94096a364f32525853fbe95aaa2ed', 'execution/source')
    need(c['NotBefore'] == '2026-10-18T14:00:00Z' and c['NotAfter'] == '2026-10-18T14:40:00Z', 'window')
    need('新加坡10月19日00:40' in doc_text and '新加坡10月19日00:40' in run, 'recovery local date')
    target_cases = {'fixture/p1.csv':'P1','fixture/p2.csv':'P2','fixture/t1.csv':'T1',
                    'fixture/t2.csv':'T2','fixture/h2-a.csv':'H2','fixture/m1-a.csv':'M1A',
                    'fixture/m1-b.csv':'M1B'}
    target_count = survivor_count = 0
    for group, binding in zip(c['Groups'], b):
        need(group['Group'] == binding['group'], 'group order')
        need(group['Copies'] == [x['Copy'] for x in binding['copy_protection']], 'copy inventory/protection differs')
        for value in (binding['project'],binding['workspace'],binding['provider'],binding['incarnation'],binding['manifest_sha256']):
            need(value in text, 'template missing environment binding')
        for row in binding['copy_protection']:
            cp = row['Copy']; case = target_cases.get(cp['Key'])
            line = next((x for x in text.splitlines() if ('@'+cp['ProviderVersion']+'；copy '+cp['ID']+'；') in x), '')
            tail = (f"{cp['Namespace']}/{cp['Key']}@{cp['ProviderVersion']}；copy {cp['ID']}；DV {cp['DatasetVersionID']}；"
                    f"SHA256 {cp['ContentSHA256']}；bytes {cp['ByteSize']}；hold={row['LegalHold']}；"
                    f"retention={row['Retention']}；retain-until={row['RetainUntil']}")
            need(tail in line, 'protection/content line differs '+cp['ID'])
            if case:
                target_count += 1
                plan_case = 'M1' if case.startswith('M1') else case
                plan = next(p for p in binding['plans'] if p['case'] == plan_case)
                need(line.startswith('仅删除 '+case+' ') and ('；plan '+plan['id']+'；hash '+plan['scope_hash']) in line, 'target plan differs')
            else:
                survivor_count += 1
                need(line.startswith('必须保留 '), 'survivor classified as target')
        for kind, hashes in [('Helpers',group['ConfigSHA256']),('CaseHelpers',group['CaseConfigSHA256'])]:
            for stage, cid in group[kind].items():
                need(cid in text and hashes[stage] in text, 'owner/config omitted '+stage)
        for dv, head in group['PolicyHeads'].items():
            need(('DV '+dv+' -> '+head) in text, 'expected policy omitted')
    need((target_count,survivor_count) == (7,11), 'target/survivor membership')

    ni = load(NATIVE/'evidence-index.json')['files']
    actual = {p.relative_to(ROOT).as_posix() for p in NATIVE.rglob('*') if p.is_file() and p.name != 'evidence-index.json'}
    need(actual == set(ni), 'native CLI index membership')
    need(all(sha(ROOT/name) == value for name,value in ni.items()), 'native CLI index SHA')
    complete = load(NATIVE/'complete.json')
    need(complete['status']=='PASS_COMPLETE_FRESH_NATIVE_PREFLIGHT' and complete['candidate_sha256']==sha(PACKET/'candidate.json')
         and complete['execution']==EXECUTION and complete['versions']==18 and complete['technical_requests']==76
         and complete['native_delete']==0 and complete['send_authority']=='NONE', 'native complete summary')
    for group,binding in zip(c['Groups'],b):
        name = group['Group']; folder = NATIVE/name
        need(load(folder/'core-before.json') == load(folder/'core-after.json') == load(PACKET/name/'core-readonly.json'), 'Core changed')
        a = load(folder/'aggregation.json')
        need(a['status']=='PASS_COMPLETE_NATIVE_AGGREGATION' and a['native_mutations']==0 and a['native_delete']==0
             and a['source_Core_unchanged'] and all(x['official_equals'] for x in a['policies']), 'aggregation')
        need(load(folder/'physical-close.json')['all_closed'], 'CLI resource closure')
    ledger=load(NATIVE/'technical-read-ledger.json')
    need(ledger['total_native_or_admin_reads']==168 and ledger['preparation_native_GET_HEAD']==76
         and ledger['complete_CLI_native_GET_HEAD']==76 and ledger['bootstrap_admin_GET']==16
         and ledger['bootstrap_enable_or_disable_PUT']==ledger['native_delete']==ledger['native_mutations']==0
         and ledger['amount']=='NOT_ASSESSED' and ledger['business_provider_attempts']=='NOT_RUN', 'read ledger')

    win=load(PACKET/'profile-windows.json')
    host=load(REVIEW/'case-recovery-budget-recheck.json')
    need(len(host['cases'])==len(host['profiles'])==7 and len(host['budgets'])==3, 'actual parent profile counts')
    for row in host['cases']:
        projection=next(x for x in win['cases'] if (x['group'],x['stage'])==(row['group'],row['stage']))
        need(row['profile_sha256']==projection['config_sha256'] and row['Batch']==EXECUTION
             and row['CandidateUseUntil']==row['BaseExpiresAt']=='2026-10-18T14:40:00Z'
             and row['BaseStartedAt']=='2026-10-18T14:00:00Z' and row['actual_contract']=='PASS' and row['stopped_pid_zero'], 'actual case receipt')
    for row in host['profiles']:
        projection=next(x for x in win['recoveries'] if (x['group'],x['stage'])==(row['group'],row['stage']))
        need(row['profile_sha256']==projection['config_sha256'] and row['ValidUntil']==projection['ValidUntil']=='2026-10-18T16:40:00Z'
             and row['read_only_profile'] and not any(row[k] for k in ('send_secret_present','wire_key_present','send_permission')), 'actual recovery receipt')
        need(row['profile_sha256'] in text and row['ValidUntil'] in text, 'recovery binding omitted')
    for row in host['budgets']:
        group=next(g for g in c['Groups'] if g['Group']==row['group'])
        need(row['owner_execution']==EXECUTION and row['empty'] and set(row['mounted_cids'])==set(group['Helpers'].values()), 'actual budget receipt')
    physical=load(REVIEW/'physical-close-recheck.json')
    need(physical['count']==len(physical['containers'])==248 and physical['permissions_disabled']==160
         and all(all(row[k] for k in ('stopped_pid_zero','no_ports','no_restart')) for row in physical['containers'])
         and physical['container_starts']==physical['native_delete']==physical['provider_io']==0 and physical['not_authorized'], 'parent physical closure receipt')
    auth=load(REVIEW/'authorization-and-schedule.json')
    need(auth['new_human_authorization']=='NOT_RECEIVED' and auth['new_schedule']=='NOT_CREATED' and not auth['old_authority_reused'], 'new authorization state')
    for row in auth['old_schedules']:
        path=Path(r'C:\Users\Henry\.codex\automations')/row['id']/'automation.toml'
        need(sha(path)==row['toml_sha256'] and tomllib.loads(path.read_text(encoding='utf-8'))['status']=='PAUSED', 'old scheduler changed')
    need(not (BASE/'delete-confirmed-v21-execution01/human-authorization.json').exists(), 'unexpected new authorization')
    support=REVIEW/'composition/window_support.py'
    tests=REVIEW/'composition/test_window_support.py'
    support_text=support.read_text(encoding='utf-8')
    for value in (ANCHOR,EXECUTION,sha(PACKET/'candidate.json'),sha(PACKET/'bindings.json'),sha(PACKET/'confirmation-template.txt')):
        need(value in support_text,'host support binding')
    need('START = dt.datetime(2026, 10, 18, 14,' in support_text, 'support fixed window')
    need(sha(support) in doc_text and sha(support) in run and sha(tests) in doc_text and sha(tests) in run, 'support doc SHA')
    entry=ROOT/'examples/lifecycle-d1/verify-remaining-native.py'
    need(sha(entry) in run and sha(entry) in doc_text and '0e0a2fd22376135cefb1b22505497f2b6e7576af43daf1562c921e2a488521f7' not in run
         and '39个11 owner' in run and sha(run_path) in doc_text, 'RB1 runbook fix or cross-reference')
    fix=load(REVIEW/'review-fix-RB1.json')
    need(sha(REVIEW/'history/runbook-before-RB1.md')==fix['before_runbook_sha256'] and sha(run_path)==fix['after_runbook_sha256'], 'RB1 failure evidence')
    result={'status':'PASS_INDEPENDENT_CANDIDATE11_EXACT_BINDING_AUDIT','at':dt.datetime.now(dt.timezone.utc).isoformat(),
            'packet_anchor':ANCHOR,'execution':EXECUTION,'raw_working_and_git_blobs_checked':len(entries),
            'native_cli_files_checked':len(ni),'targets':target_count,'survivors':survivor_count,
            'parent_live_receipts_reviewed_not_reexecuted':{'CIDs':248,'permissions':160,'cases':7,'recoveries':7,'empty_budgets':3},
            'native_technical_read_ledger':168,'reviewer_provider_io':0,'reviewer_docker_io':0,
            'authorization':'NOT_RECEIVED','new_schedule':'NOT_CREATED','old_schedules':'PAUSED',
            'RB1':'CLOSED_BY_INDEPENDENT_BYTE_AND_REFERENCE_RECHECK'}
    (OUT/'candidate11-exact-binding-audit.json').write_text(json.dumps(result,ensure_ascii=True,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(result))

if __name__=='__main__':
    audit()
