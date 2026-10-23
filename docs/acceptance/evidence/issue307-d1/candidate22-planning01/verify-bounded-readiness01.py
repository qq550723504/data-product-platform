"""File-only convergence checks; no GitHub mutation or Docker access."""
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[5]
OLD = Path('C:/Users/Henry/.codex/worktrees/d1-c21/data-product-platform')
OLD_PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')
OUT = Path(__file__).with_name('bounded-readiness01')


def need(ok, message):
    if not ok:
        raise ValueError(message)


def save(path, value):
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(json.dumps(value, indent=2)+'\n')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    OUT.mkdir(exist_ok=False)
    methods = ('test_execution_freeze', 'test_remaining_six_window', 'test_preallocation_ci')
    results = []
    for mode in ('normal', 'optimized'):
        command = [sys.executable, '-X', 'utf8', '-B']
        if mode == 'optimized':
            command.append('-O')
        command += ['-m', 'unittest', *methods]
        result = subprocess.run(command, cwd=ROOT/'examples/lifecycle-d1', capture_output=True)
        for name in ('stdout', 'stderr'):
            with (OUT/(mode+'.'+name)).open('xb') as stream:
                stream.write(getattr(result, name))
        need(result.returncode == 0 and b'Ran 21 tests' in result.stderr and result.stderr.rstrip().endswith(b'OK'), 'existing bounded regression failed')
        results.append(dict(mode=mode, tests=21, exit=result.returncode))
    # Check the existing terminal barrier against the real appended old fact.
    # A load() call would prove this path resumed past its terminal barrier.
    sys.path.insert(0, str(OLD/'examples/lifecycle-d1'))
    spec = importlib.util.spec_from_file_location('closed_c21_runtime', OLD/'examples/lifecycle-d1/six_candidate_runtime.py')
    runtime = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(runtime)
    with patch.object(runtime, 'load', side_effect=AssertionError('terminal reached allocation I/O')):
        try:
            runtime.allocation_state()
        except ValueError as error:
            need(str(error) == 'preparation already terminal; retain only, never resume', 'unexpected terminal rejection')
        else:
            raise ValueError('old preparation unexpectedly resumed')
    public = OLD/'docs/acceptance/evidence/issue307-d1/candidate21-expiry01/terminal.json'
    need(sha(public) == sha(OLD_PRIVATE/'preparation-terminal.json') ==
         '5568708e62c4ad37f8765aba667697de547d83fe0e598d59d7d4341a84b4b188', 'terminal cross-binding drift')
    current = json.loads(Path(__file__).with_name('preparation-intent01.json').read_bytes())
    need(current['execution'] != runtime.p.EXECUTION and current['not_before'] == '2026-10-23T10:00:00Z'
         and current['not_after'] == '2026-10-23T10:40:00Z'
         and current['human_confirmation'] == 'NOT_RECEIVED', 'new planning boundary differs')
    value = dict(status='PASS_EXISTING_BOUNDARIES_AND_OLD_TERMINAL_REJECTION', results=results,
                 old_resume='DENIED_BEFORE_ALLOCATION_READ', new_execution=current['execution'],
                 candidate22_runtime='NOT_IMPLEMENTED_NOT_FROZEN', source_start=0, native_delete=0,
                 branch_unlock=0, new_resource_allocation=0, full_D1='NOT_ACCEPTED')
    save(OUT/'result.json', value)
    print(json.dumps(value))


if __name__ == '__main__':
    main()
