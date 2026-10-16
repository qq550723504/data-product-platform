"""Finite offline closeout audit. Reads public receipts/Git; never a runtime entry."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[5]
OUT = Path(__file__).resolve().parent
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
REVIEWED = 'f1b03843ce26929d10cb71e744082ba77b2ebdda'
EXECUTION = '9ea8f3e2-c805-44b4-b133-a36f50df9d15'
PLAN = '8f0e9340-18f0-4a73-bdd6-622196a9011a'
RUN = BASE / 'delete-confirmed-v15-execution02'


def need(ok, message):
    if not ok:
        raise ValueError(message)


def load(path):
    return json.loads(path.read_bytes())


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def command(args):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, check=True)
    return result.stdout


def write(name, value):
    with (OUT / name).open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(value, stream, ensure_ascii=True, indent=2)
        stream.write('\n')


def main():
    # Historical provenance is fixed; final documentation commits do not change it.
    with zipfile.ZipFile(io.BytesIO(command(['git', 'archive', '--format=zip', REVIEWED]))) as archive:
        indexed = {}
        counts = {}
        for rel in ['lc13-continuation01/evidence-index-02.json',
                    'delete-candidate05-preparation01/evidence-index.json',
                    'delete-candidate05-review01/evidence-index.json',
                    'delete-confirmed-v15-execution02/evidence-index.json']:
            index = BASE / rel
            rows = load(index)['files']
            for row in rows:
                path = ROOT / row['path'] if row['path'].startswith(('docs/', 'examples/')) else index.parent / row['path']
                raw = path.read_bytes()
                need(len(raw) == row['bytes'] and digest(raw) == row['sha256'], 'Indexed bytes drift: ' + row['path'])
                git_path = path.relative_to(ROOT).as_posix()
                need(archive.read(git_path) == raw, 'Reviewed Git bytes drift: ' + git_path)
                indexed[git_path] = row['sha256']
            counts[rel] = len(rows)
        binding = load(BASE / 'delete-candidate05-preparation01/source-binding.json')
        normalized_git_sources = 0
        for path, expected in binding['runtime_source_lf_sha256'].items():
            need(digest((ROOT / path).read_bytes().replace(b'\r\n', b'\n')) == expected,
                 'Current runtime differs from frozen candidate: ' + path)
            committed = archive.read(path)
            normalized_git_sources += int(b'\r\n' in committed)
            need(digest(committed.replace(b'\r\n', b'\n')) == expected, 'Reviewed runtime source differs: ' + path)
    # Reuse the existing strict checker, preserving its preparation-snapshot label.
    prepared = json.loads(command(['python', '-X', 'utf8', 'examples/lifecycle-d1/check-reference-candidate05.py']))
    need(prepared['old_raw'] == 344 and prepared['runtime'] == 317, 'Historical candidate check changed')
    before = load(RUN / 'ledger-before.json')
    after = load(RUN / 'post-seal-audit03/final-ledger.json')
    preserved = {}
    for key, rows in before.items():
        identity = 'attempt_id' if key == 'claims' else 'id'
        actual = {row[identity]: row for row in after[key] or []}
        need(all(actual.get(row[identity]) == row for row in rows or []), 'Historical ledger row changed: ' + key)
        preserved[key] = len(rows or [])
    need({x['dispatch_id']: x for x in after['releases']} ==
         {x['dispatch_id']: x for x in load(RUN / 'old-dispatch-releases-before.json')}, 'Historical release changed')
    sent = {stage: load(RUN / ('send-' + stage) / ('send-' + stage + '-returned.json')) for stage in ('a', 'b')}
    expected = {x['attempt']['ID']: x for x in sent.values()}
    old_ids = {x['id'] for x in before['attempts']}
    new = {x['id']: x for x in after['attempts'] if x['id'] not in old_ids}
    need(set(new) == set(expected), 'More than two new attempts')
    need(len([x for x in after['claims'] if x['attempt_id'] in expected]) == 2, 'Send claim count differs')
    for attempt, result in expected.items():
        need(new[attempt]['copy_id'] == result['copy']['ID'] and new[attempt]['plan_id'] == PLAN, 'Attempt scope drift')
        obs = [x for x in after['observations'] if x['attempt_id'] == attempt]
        need(len(obs) == 1 and obs[0]['certainty'] == 'CONFIRMED' and obs[0]['outcome'] == 'UNKNOWN', 'Original UNKNOWN changed')
        stage = 'a' if result == sent['a'] else 'b'
        need(load(RUN / ('send-' + stage) / 'sender-exit.json')['exit'] == 93, 'Crash receipt differs')
    calls = {x['id'] for x in after['storage_calls']}
    allocations = [x for x in after['allocations'] if x['lifecycle_storage_call_id'] is not None]
    need(len(calls) == len(after['storage_observations']) == len(allocations) == 157, 'Storage ledger cardinality differs')
    need(calls == {x['call_id'] for x in after['storage_observations']} ==
         {x['lifecycle_storage_call_id'] for x in allocations}, 'Storage identity closure differs')
    costs = {x['id']: x for x in after['costs']}
    need(len(costs) == 160, 'Typed invocation cost count differs')
    for allocation in after['allocations']:
        activity = allocation['lifecycle_storage_call_id'] or allocation['lifecycle_provider_attempt_id']
        cost = costs[allocation['cost_event_id']]
        need(cost['activity_id'] == activity and cost['quantity'] == 1 and cost['unit'] == 'invocation', 'Typed cost mismatch')
    need(len(after['receipts']) == 2 and len(after['completion']) == 1, 'Scope completion differs')
    from datetime import datetime
    receipt_a = next(x for x in after['receipts'] if x['copy_id'] == sent['a']['copy']['ID'])
    need(datetime.fromisoformat(receipt_a['confirmed_at']) < datetime.fromisoformat(new[sent['b']['attempt']['ID']]['started_at']),
         'B did not follow durable A absence')
    native = load(RUN / 'survivors/four-survivors.json')
    baseline = {x['copy']['ID']: x for x in load(RUN / 'preflight/native/native-six-version.json')['versions']}
    targets = {x['copy']['ID'] for x in sent.values()}
    objects = {x['copy']['ID']: x for x in native['objects']}
    need(set(objects) == set(baseline) - targets and len(objects) == 4, 'Survivor scope differs')
    for copy, value in objects.items():
        previous = baseline[copy]
        need(value['copy'] == previous['copy'] and value['bytes'] == previous['bytes'] and value['sha256'] == previous['sha256'], 'Survivor bytes/hash drift')
        for key in ('LegalHold', 'RetentionMode', 'RetainUntil'):
            need(value['protection'][key] == previous['protection'][key], 'Survivor protection drift')
    topology = load(RUN / 'final-topology-closure.json')
    states = topology['all_project_containers']
    need(len(states) == len({x['id'] for x in states}) == 113 and all(len(x['id']) == 64 and x['pid'] == 0 and not x['host_ports'] and x['status'] in ('created', 'exited') for x in states), 'Canonical seal differs')
    need(load(RUN / 'admin-emergency-disable/admin-result.json')['disabled'], 'Native account seal missing')
    need(load(RUN / 'independent-final-review.json')['effective_blockers'] == 0, 'Prior independent review has blockers')
    historical = load(RUN / 'post-seal-audit03/historical-subset-check.json')
    need(len(historical['rows']) == 194 and all(x['old_rows_preserved'] for x in historical['rows']), 'Historical audit coverage missing')
    summary = {'status': 'PASS_FINITE_FINAL_CLOSURE_AUDIT', 'reviewed_source_head': REVIEWED,
               'scope': 'PR318 finite reference-demand, LC13 and authorized candidate05/v15 acceptance; not full Issue307 D1/D2',
               'indexed_payload_counts': counts, 'unique_indexed_public_payloads': len(indexed),
               'old_historical_raw_checked_by_existing_cli': 344, 'frozen_runtime_files': 317,
               'runtime_git_crlf_normalized_to_explicit_lf_contract': normalized_git_sources,
               'new_physical_attempts': 2, 'new_storage_calls': len(calls) - len(before['storage_calls']),
               'storage_calls_observations_typed_costs': 157, 'total_typed_invocation_costs': 160,
               'preserved_old_ledger_rows': preserved, 'absence_receipts': 2, 'scope_completion': 1,
               'survivors': 4, 'canonical_sealed_container_receipt': 113,
               'prior_independent_review': 'PASS_BLOCKERS0',
               'review_mode': 'Primary final closeout, public receipt/Git recomputation; not a new independent native replay',
               'provider_io': 0, 'db_io': 0, 'docker_access': 0, 'container_starts': 0,
               'delete': 0, 'merge': 0, 'cleanup': 0, 'draft': True}
    write('offline-audit.json', summary)
    print(json.dumps(summary, ensure_ascii=True))


if __name__ == '__main__':
    main()
