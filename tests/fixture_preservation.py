"""Read-only Docker preservation checks for the existing isolated IAM fixtures.

Only Mounts list order is irrelevant. Every mount field and duplicate remains
part of the comparison; diagnostics contain field names and hashes, never raw
inspect output, host paths, volume names, Config, credentials or daemon errors.
"""
import collections
import hashlib
import json
import re
import subprocess

RETAINED = {
    'original': 'dpp-agent-q2-test-postgres-20261023',
    'management': 'dpp-management-readonly-20261025',
    'runtime': 'dpp-runtime-logs-readonly-20261026',
    'main-service': 'dpp-management-main-service-20261026',
    'main-service-compat': 'dpp-management-main-service-compat-20261026',
    'directory-product': 'dpp-management-directory-product-20261026',
}


class PreservationError(RuntimeError):
    pass


def _error_code(error):
    allowed = {'INVALID_SNAPSHOT', 'INSPECT_UNAVAILABLE', 'INVALID_RESOURCE_NAME', 'INSPECT_AMBIGUOUS', 'INVALID_INSPECT_RESPONSE', 'INSPECT_IDENTITY_MISMATCH'}
    return str(error) if isinstance(error, PreservationError) and str(error) in allowed else 'OBSERVATION_ERROR'


def _json(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), allow_nan=False)


def _hash(value):
    return hashlib.sha256(_json(value).encode('utf-8')).hexdigest()


def canonical(snapshot):
    if snapshot is None:
        return None
    fields = {'id', 'status', 'mounts', 'startedAt'}
    if not isinstance(snapshot, dict) or set(snapshot) not in (fields, fields | {'image'}):
        raise PreservationError('INVALID_SNAPSHOT')
    if any(not isinstance(snapshot[k], str) or not snapshot[k] for k in fields - {'mounts'}):
        raise PreservationError('INVALID_SNAPSHOT')
    if 'image' in snapshot and (not isinstance(snapshot['image'], str) or not snapshot['image']):
        raise PreservationError('INVALID_SNAPSHOT')
    mounts = snapshot['mounts']
    if not isinstance(mounts, list) or any(not isinstance(m, dict) for m in mounts):
        raise PreservationError('INVALID_SNAPSHOT')
    for m in mounts:
        if not isinstance(m.get('Type'), str) or not m['Type'] or not isinstance(m.get('Destination'), str) or not m['Destination'] or type(m.get('RW')) is not bool:
            raise PreservationError('INVALID_SNAPSHOT')
    try:
        # JSON canonicalization preserves unknown/nested mount fields and types.
        encoded = [_json(m) for m in mounts]
        result = {**snapshot, 'mounts': [json.loads(m) for m in sorted(encoded)]}
        _json(result)
        return result
    except (ValueError, TypeError, RecursionError):
        raise PreservationError('INVALID_SNAPSHOT') from None


def _command(args, run):
    try:
        p = run(args, capture_output=True, encoding='utf-8', timeout=10)
    except (OSError, subprocess.TimeoutExpired, UnicodeError):
        raise PreservationError('INSPECT_UNAVAILABLE') from None
    if p.returncode != 0 or not isinstance(p.stdout, str) or len(p.stdout) > 8 * 1024 * 1024:
        raise PreservationError('INSPECT_UNAVAILABLE')
    return p.stdout


def container_metadata(name, *, include_image=False, run=subprocess.run):
    if not isinstance(name, str) or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,127}', name):
        raise PreservationError('INVALID_RESOURCE_NAME')
    # Only a successful exact-name listing proves absence. A failed inspect,
    # daemon outage or permission denial is never converted into "not found".
    listed = _command(['docker', 'container', 'ls', '--all', '--no-trunc', '--filter', 'name=^/' + re.escape(name) + '$', '--format', '{{.ID}}'], run).strip()
    if not listed:
        return None
    if not re.fullmatch(r'[0-9a-f]{64}', listed):
        raise PreservationError('INSPECT_AMBIGUOUS')
    raw = _command(['docker', 'container', 'inspect', listed], run)
    try:
        values = json.loads(raw)
        if not isinstance(values, list) or len(values) != 1:
            raise PreservationError('INVALID_INSPECT_RESPONSE')
        d = values[0]
        if d['Id'] != listed or d['Name'] != '/' + name:
            raise PreservationError('INSPECT_IDENTITY_MISMATCH')
        snapshot = {'id': d['Id'], 'status': d['State']['Status'], 'mounts': d['Mounts'], 'startedAt': d['State']['StartedAt']}
        if include_image:
            snapshot['image'] = d['Image']
        canonical(snapshot)
        return snapshot
    except (KeyError, IndexError, TypeError, ValueError, RecursionError):
        raise PreservationError('INVALID_INSPECT_RESPONSE') from None


def _summary(snapshot):
    return {'present': snapshot is not None, 'sha256': _hash(snapshot), 'mountCount': len(snapshot['mounts']) if snapshot is not None else 0}


def compare(before, after):
    b, a = canonical(before), canonical(after)
    changed = []
    if (b is None) != (a is None):
        changed = ['presence']
    elif b is not None:
        changed = [k for k in sorted(set(b) | set(a)) if k not in b or k not in a or _json(b[k]) != _json(a[k])]
    report = {'changedFields': changed, 'before': _summary(b), 'after': _summary(a), 'mountOrderOnly': not changed and _json(before) != _json(after)}
    if b is not None and a is not None and 'mounts' in changed:
        bm, am = collections.Counter(_json(m) for m in b['mounts']), collections.Counter(_json(m) for m in a['mounts'])
        # Per-entry hashes with multiplicity show what changed without paths or
        # extension-field names/values being copied into logs or artifacts.
        for label, entries in [('removedMounts', bm - am), ('addedMounts', am - bm)]:
            report[label] = [{'sha256': _hash(json.loads(m)), 'count': count} for m, count in sorted(entries.items())]
    return report


def _write(path, report):
    try:
        path.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    except OSError:
        raise PreservationError('DIAGNOSTIC_UNAVAILABLE') from None


def capture_retained(aliases, path, *, observer=container_metadata):
    if not aliases or len(set(aliases)) != len(aliases) or any(a not in RETAINED for a in aliases):
        raise PreservationError('INVALID_RETAINED_SET')
    before = {}
    for alias in aliases:
        try:
            before[alias] = observer(RETAINED[alias])
            canonical(before[alias])
        except Exception as e:
            _write(path, {'schemaVersion': 'fixture-preservation.v1', 'verdict': 'UNKNOWN', 'phase': 'before', 'resource': alias, 'errorCode': _error_code(e)})
            raise PreservationError('RETAINED_OBSERVATION_UNAVAILABLE') from None
    return before


def verify_retained(before, path, *, observer=container_metadata):
    if not before or any(a not in RETAINED for a in before):
        raise PreservationError('INVALID_RETAINED_SET')
    report = {'schemaVersion': 'fixture-preservation.v1', 'verdict': 'PASS', 'resources': []}
    for alias, snapshot in before.items():
        try:
            item = compare(snapshot, observer(RETAINED[alias]))
        except Exception as e:
            report.update(verdict='UNKNOWN', phase='after', resource=alias, errorCode=_error_code(e))
            _write(path, report)
            raise PreservationError('RETAINED_OBSERVATION_UNAVAILABLE') from None
        report['resources'].append({'resource': alias, **item})
        if item['changedFields']:
            report['verdict'] = 'FAIL'
    _write(path, report)
    if report['verdict'] != 'PASS':
        raise PreservationError('RETAINED_CONTAINER_CHANGED')
    return report
