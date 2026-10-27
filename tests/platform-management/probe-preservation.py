"""Repeated, read-only observations of the already-owned management fixture."""
import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tests'))
from fixture_preservation import PreservationError, _hash, _write, capture_retained, verify_retained


def main():
    if os.environ.get('MANAGEMENT_FIXTURE_ACK') != 'ISOLATED_READONLY_MANAGEMENT':
        raise PreservationError('EXPLICIT_READ_ONLY_PROBE_REQUIRED')
    run = ROOT / '.data/platform-management-preservation-20261026'
    run.mkdir(mode=0o700)  # A previous witness is never silently overwritten.
    path = run / 'retained-order-probe.json'
    before = capture_retained(('management',), path)
    if before['management'] is None:
        _write(path, {'schemaVersion': 'fixture-preservation.v1', 'verdict': 'UNKNOWN', 'errorCode': 'MISSING_PROBE_TARGET'})
        raise PreservationError('MISSING_PROBE_TARGET')
    raw, normalized = {_hash(before['management'])}, set()
    order_only = 0
    for _ in range(64):
        observed = capture_retained(('management',), path)
        report = verify_retained(before, path, observer=lambda _: observed['management'])
        raw.add(_hash(observed['management']))
        normalized.add(report['resources'][0]['after']['sha256'])
        order_only += int(report['resources'][0]['mountOrderOnly'])
    report['probe'] = {'samples': 65, 'rawVariants': len(raw), 'canonicalVariants': len(normalized), 'orderOnlySamples': order_only}
    _write(path, report)
    print(json.dumps({'verdict': report['verdict'], **report['probe']}))


if __name__ == '__main__':
    main()
