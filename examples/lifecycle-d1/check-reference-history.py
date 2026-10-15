"""Validate historical candidate04 receipts against their frozen Git runtime.

This is an offline CI harness, never a native operator or an authorization gate.
Current source stays in place and is tested by go-platform. Historical checkers
keep their strict hash checks inside an isolated public-only snapshot instead of
requiring current application code to remain forever at source055aa1df.
"""
import argparse
import hashlib
import io
import json
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path, PurePosixPath

ROOT = Path(__file__).resolve().parents[2]
BASE = 'docs/acceptance/evidence/issue307-d1/'
HISTORY = '2babc930d46c38a3c5241e52b15a13706f081345'
SOURCE = '055aa1df063e560c6796e92dfff4e03c7b9061e0'
BINDING = BASE + 'lc13-continuation01/delete-candidate04/source-binding.json'
RUNTIME = BASE + 'delete-proposal-v6/runtime-source-lf.json'
NATIVE = BASE + 'delete-readonly-v13-refresh01/'
CHECKS = (
    ('check-reference-proposal.py', 'test_reference_proposal.py'),
    ('check-reference-refresh.py', 'test_reference_refresh.py'),
    *((f'check-reference-v{n}.py', f'test_reference_v{n}.py') for n in range(8, 15)),
)


def require(ok, message):
    if not ok:
        raise ValueError(message)


def archive(root, revision, path):
    # Fixed revisions/path roots only; no shell, fetch, worktree or checkout.
    require((revision, path) in {(HISTORY, BASE.rstrip('/')), (SOURCE, 'apps/platform')},
            'Unsupported historical archive')
    return subprocess.run(['git', 'archive', '--format=zip', revision, path],
                          cwd=root, capture_output=True, check=True).stdout


def current(root, path):
    target = (root / path).resolve()
    require(target.is_relative_to(root.resolve()), 'Repository path escaped')
    return target.read_bytes()


def build_snapshot(root, destination):
    require(not destination.exists(), 'Snapshot destination must be new')
    with zipfile.ZipFile(io.BytesIO(archive(root, HISTORY, BASE.rstrip('/')))) as history:
        for path in (BINDING, RUNTIME):
            require(current(root, path) == history.read(path),
                    'Historical checkout artifact changed: ' + path)
        binding = json.loads(history.read(BINDING))
        runtime = json.loads(history.read(RUNTIME))
        require(binding['source_commit'] == runtime['source_commit'] == SOURCE
                and len(runtime['files']) == 317
                and set(runtime['files']) == set(binding['runtime_source_hashes']),
                'Frozen runtime membership drift')
        preservation = json.loads(history.read(NATIVE + 'preflight-host.json'))
        groups = [x['directory'] + '/' for x in preservation['historical_preservation']]
        groups += [NATIVE, BASE + 'delete-proposal-v14-review01/']
        paths = set()
        for group in groups:
            index = group + 'evidence-index.json'
            paths.add(index)
            for entry in json.loads(history.read(index))['files']:
                raw = history.read(entry['path'])
                require(len(raw) == entry['bytes']
                        and hashlib.sha256(raw).hexdigest() == entry['sha256'],
                        'Historical Git index drift')
                paths.add(entry['path'])
        require(len(paths) == 255, 'Historical evidence membership drift')
        for path in paths:
            require(current(root, path) == history.read(path),
                    'Historical checkout artifact changed: ' + path)

    # Copy public evidence and current Python tests/checkers. No private inputs,
    # credentials, retained data, executable binary or Docker socket is copied.
    destination.mkdir(parents=True)
    shutil.copytree(root / BASE, destination / BASE)
    scripts = destination / 'examples/lifecycle-d1'
    scripts.mkdir(parents=True)
    for script in (root / 'examples/lifecycle-d1').glob('*.py'):
        shutil.copyfile(script, scripts / script.name)

    with zipfile.ZipFile(io.BytesIO(archive(root, SOURCE, 'apps/platform'))) as source:
        for path, digest in runtime['files'].items():
            relative = PurePosixPath(path)
            require(not relative.is_absolute() and '..' not in relative.parts
                    and relative.parts[:2] == ('apps', 'platform'),
                    'Frozen source path escaped')
            raw = source.read(path)
            text = raw.replace(b'\r\n', b'\n')
            lf = hashlib.sha256(text).hexdigest()
            crlf = hashlib.sha256(text.replace(b'\n', b'\r\n')).hexdigest()
            require(lf == digest and binding['runtime_source_hashes'][path] in {lf, crlf},
                    'Frozen Git runtime differs from binding: ' + path)
            target = destination / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(raw)
    return {'historical_evidence_base': HISTORY, 'frozen_runtime_source': SOURCE,
            'historical_raw_checked': len(paths), 'runtime_source_files': 317,
            'current_source_replaced': False, 'snapshot_is_send_authorization': False}


def run_checks(snapshot):
    scripts = snapshot / 'examples/lifecycle-d1'
    for checker, test in CHECKS:
        subprocess.run([sys.executable, str(scripts / checker)], cwd=snapshot, check=True)
        for flags in ([], ['-O']):
            subprocess.run([sys.executable, *flags, '-m', 'unittest', 'discover',
                            '-s', str(scripts), '-p', test, '-v'],
                           cwd=snapshot, check=True)


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    with tempfile.TemporaryDirectory(prefix='dpp-reference-history-') as temporary:
        snapshot = Path(temporary) / 'public-snapshot'
        result = build_snapshot(ROOT, snapshot)
        run_checks(snapshot)
    result.update(status='PASS_HISTORICAL_OFFLINE_CHECKS', cli_checks=len(CHECKS),
                  test_suites=2 * len(CHECKS), provider_io=0, db_io=0,
                  container_starts=0, delete=0)
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
