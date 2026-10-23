"""Finite current-protection gate; saved equality alone never proves validity.

Pure file/value operations only. No Docker, network, IAM enable or protection
mutation. The complete native and non-delete preparation entries share this gate.
"""
import datetime as dt
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
REQUIRED_KEYS = {'business-races': 'fixture/control-retention.csv', 'mixed': 'fixture/m1-c.csv'}
MAX_OBSERVATION_AGE = dt.timedelta(minutes=5)


def need(ok, reason):
    if not ok:
        raise ValueError('native protection required: ' + reason)


def instant(value):
    need(isinstance(value, str), 'TIMESTAMP_TYPE')
    result = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
    need(result.tzinfo is not None and result.utcoffset() == dt.timedelta(), 'TIMESTAMP_NOT_UTC')
    return result


def expected_copy(group):
    scopes = json.loads((ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json').read_bytes())
    rows = [cp for s in scopes if s['Group'] == group for cp in s['Copies']
            if cp['Key'] == REQUIRED_KEYS.get(group)]
    need(len(rows) == 1, 'REQUIRED_COPY_MEMBERSHIP')
    return rows[0]


def validate_group(group, receipt, at, until):
    """Consume the actual completed SDK receipt, not a host protection projection.

    This supplements the full bytes/inventory/IAM aggregation. It does not
    replace it and does not assert a new successful native observation itself.
    """
    need(isinstance(at, dt.datetime) and at.tzinfo is not None and at.utcoffset() == dt.timedelta(), 'AS_OF_TYPE')
    horizon = instant(until)
    need(at < horizon, 'REQUIRED_HORIZON_EXPIRED')
    verification = receipt['verification']
    need(verification['group'] == group and verification['status'] == 'PASS_EXACT_NATIVE_READONLY', 'NATIVE_RECEIPT_NOT_PASS')
    started, finished = instant(receipt['started_at']), instant(receipt['finished_at'])
    need(started <= finished <= at and at-started <= MAX_OBSERVATION_AGE, 'OBSERVATION_NOT_FRESH')
    if group not in REQUIRED_KEYS:
        need(group == 'late-hold', 'UNKNOWN_GROUP')
        return {'group': group, 'required_versions': 0, 'required_until': until}
    wanted = expected_copy(group)
    rows = [row for row in verification['copies'] if row['Copy']['ID'] == wanted['ID']]
    need(len(rows) == 1 and rows[0]['Copy'] == wanted, 'EXACT_COPY_DIFFERS')
    row = rows[0]
    need(row['LegalHold'] is False, 'LEGAL_HOLD_DIFFERS')
    need(row['Retention'] == 'GOVERNANCE' and row['RetainUntil'] is not None, 'GOVERNANCE_RETENTION_MISSING')
    expiry = instant(row['RetainUntil'])
    need(started < expiry and at < expiry, 'GOVERNANCE_RETENTION_EXPIRED')
    need(expiry >= horizon, 'GOVERNANCE_COVERAGE_SHORT')
    return {'group': group, 'copy_id': wanted['ID'], 'required_versions': 1,
            'observed_at': receipt['started_at'], 'retain_until': row['RetainUntil'],
            'required_until': until, 'status': 'PASS_CURRENT_GOVERNANCE_COVERAGE'}


def validate_all(receipts, at, until):
    need(isinstance(receipts, dict) and set(receipts) == {'business-races', 'late-hold', 'mixed'}, 'SOURCE_GROUP_MEMBERSHIP')
    rows = [validate_group(group, receipts[group], at, until) for group in ('business-races', 'late-hold', 'mixed')]
    return {'status': 'PASS_CURRENT_NATIVE_PROTECTION_COVERAGE', 'groups': rows,
            'required_versions': 2, 'required_until': until, 'source_write': False, 'native_delete': 0}
