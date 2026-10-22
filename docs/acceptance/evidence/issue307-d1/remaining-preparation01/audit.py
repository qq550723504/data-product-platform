"""Offline, finite PG preparation audit. No DB, Docker or provider operations."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[4]
BASE = 'b823eb33f8f3f923b619feda06e0ecc53514dd23'
TEST = 'apps/platform/internal/lifecycle/application/remaining_preparation_integration_test.go'
EXPECTED = {
    'TestRemainingProcessingPinBlocksDispositionUntilOwnerStops',
    'TestRemainingDirectDataTransferBlocksDispositionAndReplay',
    'TestRemainingLateHoldRevokesPlanBeforeAndAfterClaim',
    'TestRemainingThreeCopyMixedOutcomesNeverCompleteRetainedScope',
    'TestDispositionHoldAndDispatchSerializeBothOrders',
    'TestOccupancyReaderAndRestrictionSerializeBothOrders',
    'TestDirectIssuedAndTransferPinCommitTogetherBeforeAnyBytes',
    'TestExactSendClaimSerializedAndLateRevocationKeepsCost',
    'TestRecoveryUnknownPresenceNeverResendsOrReleases',
}
UNCHANGED = [
    'owned_processing.go', 'occupancy.go', 'disposition.go', 'exact_execution.go',
    'recovery.go', 'storage_journal.go', 'fixture_references.go',
    'reader_integration_test.go', 'subject_pins_integration_test.go',
    'prepare_plan_integration_test.go', 'disposition_integration_test.go',
    'occupancy_integration_test.go', 'exact_execution_integration_test.go',
    'recovery_integration_test.go',
]


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read(name):
    return json.loads((HERE / name).read_text(encoding='utf-8'))


def result():
    process = read('pg-race-attempt05-process.json')
    events = [json.loads(line) for line in (HERE / 'pg-race-attempt05.jsonl').read_text(encoding='utf-8').splitlines()]
    assert process['exit_code'] == 0 and process['race'] is True
    assert process['native_provider_configured'] is False
    assert not any(e['Action'] in {'skip', 'fail'} for e in events)
    assert events[-1]['Action'] == 'pass' and 'Test' not in events[-1]
    passed = {e['Test'] for e in events if e['Action'] == 'pass' and 'Test' in e}
    assert {t for t in passed if '/' not in t} == EXPECTED
    assert len(passed) == 17
    assert (HERE / 'pg-race-attempt05-stderr.txt').read_bytes() == b''
    assert digest((ROOT / TEST).read_bytes()) == process['test_source_sha256']
    for attempt in ('01', '02', '04'):
        assert read(f'pg-race-attempt{attempt}-process.json')['exit_code'] != 0
        assert any(json.loads(line)['Action'] == 'fail' for line in (HERE / f'pg-race-attempt{attempt}.jsonl').read_text(encoding='utf-8').splitlines())
    for attempt in ('01', '02', '03'):
        assert read(f'operator-attempt{attempt}-failure.json')['status'] == 'FAIL'
    assert read('pg-race-attempt03-process.json')['exit_code'] == 0
    assert read('static-vet.json')['exit_code'] == 0
    start, seal = read('scratch-start.json'), read('scratch-seal-final.json')
    assert start['native_provider_configured'] is False and start['writable_named_volume'] is True
    assert seal['container_id'] == start['container_id'] and seal['running'] is False and seal['pid'] == 0
    assert seal['delete'] == seal['provider_io'] == seal['cleanup'] == 0
    assert not any(v for v in (seal.get('ports') or {}).values())
    assert all(not c['running'] and c['pid'] == 0 for c in seal['retained_5d_read_only_inspect'])
    provenance = []
    for name in UNCHANGED:
        path = 'apps/platform/internal/lifecycle/application/' + name
        disk = (ROOT / path).read_bytes()
        base = subprocess.check_output(['git', 'show', BASE + ':' + path], cwd=ROOT)
        assert disk.replace(b'\r\n', b'\n') == base.replace(b'\r\n', b'\n'), path
        provenance.append({'path': path, 'checkout_sha256': digest(disk), 'base_blob_sha256': digest(base), 'comparison': 'CRLF-to-LF only'})
    return {
        'scope': 'remaining-D1 PG-only composition preparation', 'status': 'PASS',
        'base_commit': BASE, 'new_test_path': TEST,
        'new_test_sha256': process['test_source_sha256'],
        'top_level_tests': 9, 'leaf_tests': 13, 'parent_and_child_pass_events': len(passed),
        'failed_tests': 0, 'skipped_tests': 0, 'race': True,
        'native_acceptance': 'NOT_RUN', 'native_candidates': 'NOT_PROVISIONED',
        'delete_authorization': 'NOT_AUTHORIZED', 'delete_budget': 0,
        'full_D1': 'NOT_ACCEPTED', 'issue_307': read('remote-readback.json'),
        'unchanged_baseline_sources': provenance,
        'scope_limits': ['synthetic dependencies/gate/port/observations', 'not actual queue/TCP/provider acceptance', 'not a new independent review or current-head CI'],
    }


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--write', action='store_true', help='Create new audit/index once; never overwrite.')
    args = parser.parse_args()
    audit = encoded(result())
    if args.write:
        with (HERE / 'preparation-audit.json').open('xb') as stream:
            stream.write(audit)
        index = {'files': [{'path': p.name, 'bytes': p.stat().st_size, 'sha256': digest(p.read_bytes())} for p in sorted(HERE.iterdir()) if p.is_file() and p.name != 'evidence-index.json']}
        with (HERE / 'evidence-index.json').open('xb') as stream:
            stream.write(encoded(index))
    else:
        assert (HERE / 'preparation-audit.json').read_bytes() == audit
    index = read('evidence-index.json')
    assert {p.name for p in HERE.iterdir() if p.is_file()} == {r['path'] for r in index['files']} | {'evidence-index.json'}
    for item in index['files']:
        data = (HERE / item['path']).read_bytes()
        assert len(data) == item['bytes'] and digest(data) == item['sha256'], item['path']
    print(json.dumps({'status': 'PASS', 'top_level_tests': 9, 'skip': 0, 'delete_budget': 0, 'indexed_files': len(index['files'])}))
