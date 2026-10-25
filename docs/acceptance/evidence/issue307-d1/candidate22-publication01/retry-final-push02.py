"""One normal final-artifact commit, then a separately requested normal push.

No source edit, force, branch unlock, provider call, or execution authority.
The publication journal is deliberately outside the staged preparation paths.
"""
import argparse
import datetime as dt
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[5]
BASE = ROOT/'docs/acceptance/evidence/issue307-d1'
HERE = Path(__file__).parent
PUB = BASE/'candidate22-publication01'
SOURCE = 'a50b5a6307e79896578669ed53a3e12f08a0a815'
ANCHOR = '10e39eede9c0b30f1cab1397a4a42c5698479862'
BRANCH = 'refs/heads/codex/issue307-post-v22-dispatch-closure'
sys.path.insert(0, str(ROOT/'examples/lifecycle-d1'))
import fixture_artifacts as f
import execution_freeze as ef


def save(path, value):
    with path.open('xb') as stream:
        stream.write((json.dumps(value, indent=2)+'\n').encode())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['commit', 'push'])
    args = parser.parse_args()
    out = PUB/('final-artifact-'+args.mode+'02')
    out.mkdir(parents=True, exist_ok=False)
    sequence = 0

    def run(*argv):
        nonlocal sequence
        sequence += 1
        command = ['git', '-c', 'core.longpaths=true', '-c', 'core.autocrlf=false', *argv]
        save(out/f'{sequence:02d}-intent.json', {'argv': command, 'at': dt.datetime.now(dt.timezone.utc).isoformat()})
        result = subprocess.run(command, cwd=ROOT, capture_output=True)
        for channel in ('stdout', 'stderr'):
            with (out/f'{sequence:02d}-{channel}.txt').open('xb') as stream:
                stream.write(getattr(result, channel))
        save(out/f'{sequence:02d}-result.json', {'exit': result.returncode})
        f.need(result.returncode == 0, 'Git action failed; preserve journal; no automatic force or unknown push retry')
        return result.stdout.decode('utf-8').strip()

    frozen = f.load(BASE/'remaining-candidate22/source-freeze.json')
    f.need(frozen['host']['source'] == SOURCE, 'Frozen host source differs')
    f.verify_host(ROOT, frozen['host'])
    f.need(not run('diff', '--name-only', 'HEAD'), 'Tracked preparation/source drift')
    f.need(not run('diff', '--cached', '--name-only'), 'Unexpected staged work')
    f.need(run('ls-remote', 'origin', BRANCH) == SOURCE+'\t'+BRANCH, 'Remote branch no longer at reviewed source')
    if args.mode == 'commit':
        f.need(run('rev-parse', 'HEAD') == ANCHOR, 'Local containment anchor differs')
        review = f.load(BASE/'candidate22-artifacts-review01/independent-final-review01.json')
        f.need(review['status'] == 'PASS_BOUNDED_C22_FROZEN_NONDELETE_PREPARATION_REVIEW'
               and review['remaining_blockers'] == [], 'Independent packet review incomplete')
        f.need(f.load(HERE/'phase-final-static-audit01/result.json')['exit'] == 0, 'Final same-entry audit incomplete')
        f.need(f.load(HERE/'publication-secret-scan03.json')['status'] == 'PASS_NO_ACTUAL_CREDENTIAL_BYTES', 'Final credential scan incomplete')
        paths = [p for p in BASE.iterdir() if p.is_dir() and p.name.startswith('candidate22-') and p != PUB]
        paths += [BASE/('preallocation-ci-'+suffix) for suffix in (
            '5630d5bf27d340578eb2d90d2eace4c6', 'e38e3076b2114cbab686bf95c033fe61')]
        paths += [ROOT/'docs/acceptance'/name for name in (
            'issue307-d1-candidate22-confirmation.md', 'issue307-d1-v32-window-runbook.md',
            'issue307-d1-v32-preparation.md')]
        f.need(all(p.exists() for p in paths), 'Expected preparation path missing')
        run('add', '--', *(p.relative_to(ROOT).as_posix() for p in paths))
        staged = run('diff', '--cached', '--name-only').splitlines()
        allowed = [p.relative_to(ROOT).as_posix() for p in paths]
        f.need(staged and all(any(n == p or n.startswith(p+'/') for p in allowed) for n in staged), 'Foreign staged path')
        f.need(not run('diff', '--cached', '--name-only', '--', *f.HOST_PATHS), 'Final commit changes frozen runtime source')
        run('commit', '-q', '-m', 'docs(lifecycle): freeze candidate22 non-delete evidence and exact proposal')
        head = run('rev-parse', 'HEAD')
        f.need(run('rev-parse', head+'^') == ANCHOR, 'Unexpected final parent')
        f.verify_host(ROOT, frozen['host'])
        result = {'status': 'FINAL_ARTIFACT_COMMITTED_NOT_PUSHED', 'head': head,
                  'host_source': SOURCE, 'files': len(staged), 'native_delete': 0, 'execution_authority': False}
    else:
        committed = f.load(PUB/'final-artifact-commit01/result.json')
        head = run('rev-parse', 'HEAD')
        f.need(head == committed['head'] and run('merge-base', SOURCE, head) == SOURCE, 'Final normal-FF lineage differs')
        snapshot = ef.GitHub(out).snapshot()
        ef.ready(snapshot, SOURCE)
        maintenance = f.load(HERE/'authorized-maintenance01/operation03/after.json')
        f.need(snapshot['rule']['id'] == maintenance['rule']['id'] == 'BPR_kwDOUdQLBs4FE_7b'
               and snapshot['rule'] == maintenance['rule']
               and snapshot['protection'] == maintenance['protection']
               and snapshot['rule']['lockBranch'] is False, 'Maintenance protection drift; no push')
        save(out/'protection-before-final-push.json', snapshot)
        run('push', 'origin', head+':'+BRANCH)
        f.need(run('ls-remote', 'origin', BRANCH) == head+'\t'+BRANCH, 'Final push readback differs')
        result = {'status': 'FINAL_ARTIFACT_NORMAL_FF_PUBLISHED_PENDING_EXACT_HEAD_CI',
                  'head': head, 'native_delete': 0, 'execution_authority': False}
    save(out/'result.json', result)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
