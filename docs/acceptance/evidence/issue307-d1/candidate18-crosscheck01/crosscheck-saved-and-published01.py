"""Append-only observations for the human-requested v28 crosscheck; no execution API."""
from pathlib import Path
import concurrent.futures
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
import sys

ROOT = Path.cwd()
OUT = Path(__file__).resolve().parent
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
PACKET = BASE / 'remaining-candidate18'
PRIVATE = Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate18')
HEAD = 'a8930b31aa84fe0c003e2744253358e36c2f12f7'
SOURCE = '618f484061c3624ff2195c0b591e7d8e3d743ce1'
EXECUTION = '4fd5a7e7-779e-41d7-a35a-3bab7dd615b5'

def need(ok, message):
    if not ok:
        raise ValueError(message)

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def load(path):
    return json.loads(path.read_bytes())

def raw_save(name, raw):
    with (OUT / name).open('xb') as stream:
        stream.write(raw)

def save(name, value):
    raw_save(name, (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode())

def now():
    return dt.datetime.now(dt.timezone.utc).isoformat()

def git(*args):
    return subprocess.check_output(['git', '-c', 'core.longpaths=true', *args], cwd=ROOT)

def main():
    save('intent01.json', {'started_at': now(), 'execution': EXECUTION,
         'scope': 'Crosscheck actual assistant text, frozen artifacts, exact published PR HEAD and closed host only',
         'authorization': 'NOT_RECEIVED', 'schedule': 'NOT_CREATED', 'source_start': 0,
         'provider_io': 0, 'native_delete': 0, 'modify_frozen_files': False})
    session = Path(r'C:\Users\Henry\.codex\sessions\2026\10\13\rollout-2026-10-13T23-50-22-01a13579-573d-7a32-b1e1-3297fe78380a_01a135f5-2d3a-7953-8149-c86a26446d07.jsonl')
    messages = []
    for line in session.open(encoding='utf-8'):
        event = json.loads(line); payload = event.get('payload', {})
        if event.get('type') == 'response_item' and payload.get('type') == 'message' and payload.get('role') == 'assistant':
            text = ''.join(x.get('text', '') for x in payload.get('content', []) if isinstance(x, dict))
            if text.startswith('以下是 **candidate18/v28'):
                messages.append((event, text))
    need(len(messages) == 1, 'Expected one actual complete v28 assistant text')
    event, text = messages[0]
    raw_save('assistant-confirmation-verbatim.txt', text.encode())
    blocks = re.findall(r'```(?:text)?\n(.*?)```', text, re.S)
    template = (PACKET / 'confirmation-template.txt').read_bytes()
    need(blocks[0].encode() == template, 'Pasted machine template differs from frozen bytes')
    doc = (ROOT / 'docs/acceptance/issue307-d1-candidate18-confirmation.md').read_text(encoding='utf-8')
    need(template.decode() in doc, 'Canonical confirmation document differs from machine template')
    candidate = load(PACKET / 'candidate.json'); bindings = load(PACKET / 'bindings.json')
    host = load(BASE / 'candidate18-planning01/candidate18-host-bindings01.json')
    pairs = re.findall(r'^\| `([^`]+)` \| `([0-9a-f]{64})` \|$', text, re.M)
    need(len(pairs) == 47 and len(dict(pairs)) == 47, 'Chat build/host tables differ')
    expected = dict(host['initial_build_files']); expected.update({k:v['sha256'] for k,v in host['files'].items()})
    need(dict(pairs) == expected, 'Chat 12 build and 35 host bindings differ')
    for name, digest in pairs:
        path = ROOT / name if '/' in name else PRIVATE / name
        need(sha(path.read_bytes()) == digest, 'Actual chat table member differs: ' + name)
    appendix_files = [
        'docs/acceptance/issue307-d1-candidate18-confirmation.md',
        'docs/acceptance/issue307-d1-v28-window-runbook.md',
        *['docs/acceptance/evidence/issue307-d1/' + x for x in [
            'candidate18-planning01/candidate18-host-bindings01.json',
            'candidate18-planning01/approval-reference-manifest01.json',
            'candidate18-planning01/final-static-evidence-audit-plan01.json',
            'candidate18-source-review01/independent-final-review01.json',
            'candidate18-source-review01/evidence-index.json',
            'candidate18-review01/independent-final-review01.json',
            'candidate18-review01/evidence-index.json',
            'candidate18-review01/reviewed-artifacts01.json',
            'candidate18-planning01/retained-backups48-readback01.json',
            'candidate18-planning01/publication-checkpoint01.json']]]
    for name in appendix_files:
        need(sha((ROOT / name).read_bytes()) in blocks[1], 'Chat appendix hash differs: ' + name)
    need(sha((PRIVATE / 'host-anchor.json').read_bytes()) in blocks[1], 'Private host anchor differs')
    need(candidate['Execution'] == EXECUTION and candidate['Source'] == SOURCE and HEAD in blocks[1], 'Execution/source/HEAD differs')
    sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
    import remaining_sequence as sequence
    ordered = sequence.targets(candidate)
    target_ids = {cp['ID'] for _, _, cp in ordered}
    rows = []
    for line in template.decode().splitlines():
        if not line.startswith(('删除目标 ', '保留版本 ')):
            continue
        cells = line.rstrip('。').split('；')
        label, project, exact = cells[0].split(' ')
        copy_id = cells[1].removeprefix('copy ')
        b = next(b for b in bindings if b['project'] == project)
        bound = next(x for x in b['copy_protection'] if x['Copy']['ID'] == copy_id)
        cp = bound['Copy']
        need(cells[:9] == [f'{label} {project} {cp["Namespace"]}/{cp["Key"]}@{cp["ProviderVersion"]}',
             f'copy {cp["ID"]}', f'DV {cp["DatasetVersionID"]}', f'SHA256 {cp["ContentSHA256"]}',
             f'bytes {cp["ByteSize"]}', f'hold={bound["LegalHold"]}', f'retention={bound["Retention"]}',
             f'retain-until={bound["RetainUntil"]}', cells[8]], 'Exact tuple/protection differs: ' + copy_id)
        need((label == '删除目标') == (copy_id in target_ids), 'Target/survivor classification differs')
        if cells[8].startswith('plan '):
            need(any(cells[8] == 'plan '+plan['id'] and cells[9] == 'hash '+plan['scope_hash'] for plan in b['plans']), 'Plan/hash differs')
        else:
            need(label == '保留版本' and cells[8] == '非发送目标', 'Unbound target plan')
        if copy_id in target_ids:
            need(not bound['LegalHold'] and bound['Retention'] == 'NONE' and bound['RetainUntil'] is None, 'Target unexpectedly protected')
        rows.append({'kind': 'target' if copy_id in target_ids else 'survivor', 'group': b['group'], 'project': project, **bound})
    need(len(rows) == 17 and len({x['Copy']['ID'] for x in rows}) == 17, 'Present tuple set differs')
    need([x['Copy'] for x in rows if x['kind'] == 'target'] == [x[2] for x in ordered], 'Frozen six order differs')
    native_times = []
    for b in bindings:
        native = load(BASE / 'remaining-candidate18-readonly01' / b['group'] / 'files/six-native-verification.json')
        need(native['verification']['copies'] == b['copy_protection'], 'Saved actual native tuples/protection differ')
        group = next(g for g in candidate['Groups'] if g['Group'] == b['group'])
        need([x['Copy'] for x in b['copy_protection']] == [cp for cp in group['Copies'] if cp['Key'] != 'fixture/p1.csv'], 'Candidate present copies differ')
        native_times.append({'group': b['group'], 'started_at': native['started_at'], 'finished_at': native['finished_at']})
    first = load(BASE / 'remaining-candidate18-readonly01/business-races/files/six-native-verification.json')
    need(first['p1_absence']['code'] == 'NoSuchVersion' and first['p1_absence']['status'] == 404, 'P1 absence evidence differs')
    save('text-and-tuples01.json', {'status': 'PASS_EXACT_ACTUAL_CHAT_FROZEN_TEXT_AND_17_TUPLES', 'observed_at': now(),
         'assistant_message_id': event['payload'].get('id'), 'assistant_timestamp': event.get('timestamp'),
         'assistant_text_sha256': sha(text.encode()), 'machine_block_bytes': len(template), 'machine_block_sha256': sha(template),
         'build_table_members': 12, 'host_table_members': 35, 'appendix_file_hashes': len(appendix_files),
         'execution': EXECUTION, 'order': [stage for _, stage, _ in ordered], 'targets': 6, 'survivors': 11,
         'p1': 'EXCLUDED_SAVED_NATIVE_404', 'rows': rows, 'saved_native_times': native_times,
         'new_native_preflight': 'NOT_RUN', 'execution_authority': False, 'native_delete': 0})
    need(git('rev-parse', 'HEAD').decode().strip() == HEAD, 'Local HEAD drift')
    need(not git('diff', '--name-only', 'HEAD').strip(), 'Tracked working bytes changed')
    freeze = load(PACKET / 'source-freeze.json')
    source_results = []
    for name, spec in freeze['files'].items():
        raw = (ROOT / name).read_bytes(); lf = raw.replace(b'\r\n', b'\n')
        need(sha(raw) == spec['raw_sha256'] and sha(lf) == spec['lf_sha256'], 'Frozen raw/LF source drift: '+name)
    need(not git('diff', '--name-only', SOURCE, HEAD, '--', 'apps', 'examples/lifecycle-d1', 'migrations', 'go.mod', 'go.sum').strip(), 'Published runtime differs from approved source')
    backup = load(BASE / 'candidate18-planning01/retained-backups48-readback01.json')
    for x in backup['old'] + backup['new']:
        path = PRIVATE.parent / x['private_relative_path']
        need(sha(path.read_bytes()) == x['sha256'] and path.stat().st_size == x['bytes'], 'Retained backup changed: '+str(path))
    save('source-and-backups01.json', {'status': 'PASS_FROZEN_RAW_LF_SOURCE_AND_48_BACKUPS', 'observed_at': now(),
         'head': HEAD, 'source': SOURCE, 'source_members': len(freeze['files']), 'tracked_changes': 0,
         'backups': 48, 'new': 9, 'old_complete': 36, 'old_partial': 3, 'execution_authority': False, 'native_delete': 0})
    queries = {
      'pr': ['api', 'repos/qq550723504/data-product-platform/pulls/332'],
      'issue': ['api', 'repos/qq550723504/data-product-platform/issues/307'],
      'ci-run': ['api', 'repos/qq550723504/data-product-platform/actions/runs/39577806574'],
      'ci-jobs': ['api', 'repos/qq550723504/data-product-platform/actions/runs/39577806574/jobs?per_page=100'],
      'check-runs': ['api', 'repos/qq550723504/data-product-platform/commits/'+HEAD+'/check-runs?per_page=100'],
      'threads': ['api', 'graphql', '-f', 'query={repository(owner:"qq550723504",name:"data-product-platform"){pullRequest(number:332){reviewThreads(first:100){nodes{id isResolved} pageInfo{hasNextPage endCursor}}}}}']}
    def fetch(item):
        name, args = item; start = now()
        result = subprocess.run(['gh', *args], capture_output=True, cwd=ROOT)
        raw_save(name+'-readback01.stdout.json', result.stdout); raw_save(name+'-readback01.stderr.txt', result.stderr)
        need(result.returncode == 0, 'GitHub read failed: '+name)
        return name, {'start': start, 'finished': now(), 'value': json.loads(result.stdout)}
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        remote = dict(pool.map(fetch, queries.items()))
    pr = remote['pr']['value']; issue = remote['issue']['value']; run = remote['ci-run']['value']; jobs = remote['ci-jobs']['value']
    threads = remote['threads']['value']['data']['repository']['pullRequest']['reviewThreads']
    checks = remote['check-runs']['value']
    need(pr['state'] == 'open' and pr['draft'] and not pr['merged'] and pr['head']['sha'] == HEAD and issue['state'] == 'open', 'PR/Issue/HEAD status drift')
    need(not threads['pageInfo']['hasNextPage'] and all(x['isResolved'] for x in threads['nodes']), 'Unresolved or uninspected review threads')
    need(run['head_sha'] == HEAD and run['event'] == 'pull_request' and run['status'] == 'completed' and run['conclusion'] == 'success', 'Exact HEAD CI not successful')
    need(jobs['total_count'] == len(jobs['jobs']) == 13 and all(j['status'] == 'completed' and j['conclusion'] == 'success' and j['head_sha'] == HEAD for j in jobs['jobs']), 'CI jobs incomplete or wrong HEAD')
    need(checks['total_count'] == len(checks['check_runs']) and all(c['status'] == 'completed' and c['conclusion'] == 'success' and c['head_sha'] == HEAD for c in checks['check_runs']), 'Current checks incomplete')
    save('github-checkpoint01.json', {'status': 'PASS_CURRENT_EXACT_HEAD_DRAFT_OPEN_PR_CI_AND_THREADS', 'observed_at': now(),
         'read_times': {k:{a:v[a] for a in ('start','finished')} for k,v in remote.items()}, 'head': HEAD,
         'pr': 332, 'draft': True, 'merged': False, 'issue': 307, 'issue_state': 'OPEN', 'run_id': run['id'],
         'event': run['event'], 'attempt': run['run_attempt'], 'jobs_success': len(jobs['jobs']),
         'checks_success': len(checks['check_runs']), 'unresolved_threads': 0, 'main_ci': 'NOT_CLAIMED',
         'execution_authority': False, 'native_delete': 0})
    print(json.dumps({'status': 'PASS_SAVED_AND_PUBLISHED_CROSSCHECK', 'execution': EXECUTION, 'head': HEAD,
                      'targets': 6, 'survivors': 11, 'PR_CI': '13/13 SUCCESS', 'native_delete': 0}))

if __name__ == '__main__':
    main()
