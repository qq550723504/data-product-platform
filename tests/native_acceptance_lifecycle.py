"""Explicit native smoke: new synthetic resources only; retain their data stopped."""
import json
import hashlib
from pathlib import Path
import sys
import uuid

sys.path.insert(0, str(Path(__file__).parent))
from acceptance_lifecycle import AcceptanceRun, command, container_identity, finish, need, process_identity, trial

ROOT = Path(__file__).resolve().parents[1]


def main():
    suffix = uuid.uuid4().hex[:10]
    scopes, results = [], []
    sentinel = AcceptanceRun(ROOT, ROOT / '.data' / ('shutdown-sentinel-' + suffix))
    scopes.append(sentinel)
    sentinel.begin()
    context = sentinel.state['context']
    def docker(*args):
        return command(['docker', '--context', context, *args])
    def inventory():
        ids = docker('ps', '-aq').split()
        return {x['Id']: {'identity': container_identity(x), 'state': x['State']['Status'], 'startedAt': x['State']['StartedAt']}
                for x in json.loads(docker('inspect', *ids))} if ids else {}
    original = inventory()
    old_volumes = set(docker('volume', 'ls', '-q').split())
    # This sentinel has a different owner and must stay running during both stops.
    wait = ['alpine:3.22', 'sh', '-c', 'trap "exit 0" TERM; while :; do sleep 1 & wait $!; done']
    try:
        sentinel_id = sentinel.create_container(['--name', 'dpp-shutdown-sentinel-' + suffix, '--network', 'none', *wait])
        sentinel_before = inventory()[sentinel_id]
        for mode in ('success', 'failed-acceptance'):
            scope = AcceptanceRun(ROOT, ROOT / '.data' / ('shutdown-' + mode + '-' + suffix))
            scopes.append(scope)
            cid, process, volume = None, None, 'dpp-shutdown-' + mode + '-' + suffix
            def acceptance():
                nonlocal cid, process
                scope.begin()
                need(scope.state['context'] == context, 'SMOKE_CONTEXT_CHANGED')
                cid = scope.create_container(['--name', volume, '--network', 'none',
                                              '--mount', 'type=volume,source=' + volume + ',target=/data', *wait])
                scope.docker('exec', cid, 'sh', '-c', 'printf retained-business-proof > /data/proof')
                (scope.run / 'original-evidence.txt').write_text('retained-original-evidence')
                process = scope.spawn('owned-process', [sys.executable, '-c', 'import time;time.sleep(120)'],
                                      cwd=ROOT, env=trial.environment())
                if mode == 'failed-acceptance':
                    raise RuntimeError('EXPECTED_SYNTHETIC_ACCEPTANCE_FAILURE')
            try:
                finish(scope, acceptance)
                need(mode == 'success', 'EXPECTED_FAILURE_NOT_OBSERVED')
            except RuntimeError as error:
                need(mode == 'failed-acceptance' and str(error) == 'EXPECTED_SYNTHETIC_ACCEPTANCE_FAILURE', 'UNEXPECTED_SMOKE_FAILURE')
            need(json.loads(docker('inspect', cid))[0]['State']['Status'] == 'exited', 'OWNED_CONTAINER_STILL_RUNNING')
            docker('cp', cid + ':/data/proof', str(scope.run / 'recovered-proof.txt'))
            need((scope.run / 'recovered-proof.txt').read_text() == 'retained-business-proof', 'VOLUME_DATA_CHANGED')
            need((scope.run / 'original-evidence.txt').read_text() == 'retained-original-evidence', 'EVIDENCE_CHANGED')
            need(process_identity(process['pid'], expected=process['identity']) is None, 'OWNED_PROCESS_STILL_ALIVE')
            need(inventory()[sentinel_id] == sentinel_before, 'OTHER_RUN_CHANGED')
            reports = [json.loads(p.read_text()) for p in scope.run.glob('cleanup-*.json')]
            need(reports[-1]['status'] == 'PASS', 'CLEANUP_NOT_CONFIRMED')
            results.append({'case': mode, 'status': 'PASS', 'containerId': cid, 'volume': volume,
                            'processStopped': True, 'dataAndEvidenceRetained': True, 'otherRunUnchanged': True})
    finally:
        # Finish still-open scopes; never retry a recorded ownership refusal.
        failures = []
        for scope in reversed(scopes):
            if not scope.started:
                scope.close()
                continue
            if scope.guard is None:
                receipts = [json.loads(p.read_text()) for p in scope.run.glob('cleanup-*.json')]
                if not receipts or any(r['status'] != 'PASS' for r in receipts):
                    failures.append(scope.run.name)
            else:
                try:
                    scope.stop()
                except Exception:
                    failures.append(scope.run.name)
                finally:
                    scope.close()
        need(not failures, 'NATIVE_SHUTDOWN_UNCONFIRMED_RETAIN_FAILURE_EVIDENCE')
    current = inventory()
    need(all(current.get(cid) == before for cid, before in original.items()), 'PREEXISTING_CONTAINER_CHANGED')
    need(old_volumes.issubset(set(docker('volume', 'ls', '-q').split())), 'PREEXISTING_VOLUME_REMOVED')
    report = {'status': 'PASS', 'cases': results, 'preexistingContainersUnchanged': len(original),
              'preexistingVolumesRetained': len(old_volumes), 'newRunningContainers': 0, 'deletions': 0,
              'sourceHashes': {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in
                               ('tests/acceptance_lifecycle.py', 'tests/platform-management/bootstrap.py',
                                'tests/platform-management/run.py', 'tests/platform-management/start-web.py',
                                'tests/platform-management/stop.py', 'tests/native_acceptance_lifecycle.py')}}
    (sentinel.run / 'native-results.json').write_text(json.dumps(report, indent=2), encoding='utf-8')
    print(json.dumps({'status': report['status'], 'cases': len(results),
                      'preexistingContainersUnchanged': len(original), 'newRunningContainers': 0,
                      'report': str(sentinel.run / 'native-results.json')}))


if __name__ == '__main__':
    main()
