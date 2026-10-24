"""Independent source-only checks; no Docker, DB, provider or resource commands."""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[6]
OUT = Path(__file__).resolve().parent
HEAD = '618f484061c3624ff2195c0b591e7d8e3d743ce1'
TESTS = [
    'test_retained_v27.py', 'test_six_candidate_runtime.py',
    'test_window_six_support.py', 'test_remaining_six_window.py',
    'test_freeze_six_candidate.py', 'test_retained_v26.py',
    'test_six_native_entry.py', 'test_verify_six_native.py',
    'test_evidence_indexes.py', 'test_v27_dispatch_closure.py',
    'test_post_v27_seventeen_refresh.py',
]

def git(*args):
    return subprocess.check_output(['git', '-c', 'core.longpaths=true', *args], cwd=ROOT).decode().strip()

if git('rev-parse', 'HEAD') != HEAD or git('status', '--porcelain', '--untracked-files=no'):
    raise ValueError('exact source HEAD or tracked state differs')

env = os.environ.copy()
env['GOPROXY'] = 'off'
env['GOTOOLCHAIN'] = 'local'
env['PYTHONDONTWRITEBYTECODE'] = '1'
for key in list(env):
    if key.upper() in {'DATABASE_URL', 'TEST_DATABASE_URL', 'POSTGRES_DSN', 'PG_DSN'}:
        del env[key]
commands = []
for test in TESTS:
    for optimized in (False, True):
        commands.append((ROOT, [sys.executable, *(['-O'] if optimized else []), '-B', 'examples/lifecycle-d1/' + test]))
go = [
    ('./internal/lifecycle/pilot', '^TestRemainingRetainedIdentitiesKeepActualC14C15C16AndTerminalC17Users$'),
    ('./cmd/lifecycle-fixture-remaining-iam', '^TestBootstrap(NeverExposesObjectDeletionOrUnconfirmedEnable|SelectsOnlyCurrentExactIdentityAndDormantIsNeverEnabled)$'),
    ('./cmd/lifecycle-fixture-readonly-check', '^TestSixExpectationExportKeepsHistoricalReceiptAndUsesKnownInventory$'),
]
for package, selector in go:
    commands.append((ROOT/'apps/platform', ['go', 'test', '-count=1', package, '-run', selector, '-v']))

results = []
for i, (cwd, command) in enumerate(commands, 1):
    started = dt.datetime.now(dt.timezone.utc).isoformat()
    run = subprocess.run(command, cwd=cwd, env=env, capture_output=True, timeout=180)
    receipt = {'index': i, 'command': command, 'cwd': str(cwd), 'started_at': started,
               'completed_at': dt.datetime.now(dt.timezone.utc).isoformat(), 'exit_code': run.returncode}
    for stream in ('stdout', 'stderr'):
        raw = getattr(run, stream)
        name = f'{i:02d}.{stream}.txt'
        with (OUT/name).open('xb') as target:
            target.write(raw)
        receipt[stream] = {'path': name, 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}
    match = re.search(rb'Ran (\d+) tests?', run.stderr)
    if match:
        receipt['unittest_methods'] = int(match.group(1))
    results.append(receipt)
    print(json.dumps({'index': i, 'exit_code': run.returncode, 'methods': receipt.get('unittest_methods')}), flush=True)

if git('rev-parse', 'HEAD') != HEAD or git('status', '--porcelain', '--untracked-files=no'):
    raise ValueError('source changed during independent checks')
result = {'status': 'PASS_INDEPENDENT_SOURCE_ONLY_REGRESSION' if all(r['exit_code'] == 0 for r in results) else 'FAIL',
          'source_head': HEAD, 'commands': len(results), 'results': results,
          'normal_python_methods': sum(r.get('unittest_methods', 0) for r in results[:22:2]),
          'optimized_python_methods': sum(r.get('unittest_methods', 0) for r in results[1:22:2]),
          'docker_calls': 0, 'database_calls': 0, 'provider_calls': 0, 'real_DELETE': 0}
with (OUT/'result.json').open('x', encoding='utf-8', newline='\n') as target:
    json.dump(result, target, indent=2)
    target.write('\n')
print(json.dumps({k: v for k, v in result.items() if k != 'results'}), flush=True)
sys.exit(0 if result['status'].startswith('PASS_') else 1)
