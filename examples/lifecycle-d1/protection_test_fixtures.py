"""Non-authoritative SDK receipt doubles for file/composition regressions only."""
import datetime as dt
import native_protection as protection


def receipt(group, now, until):
    rows = []
    if group in protection.REQUIRED_KEYS:
        rows.append({'Copy': protection.expected_copy(group), 'LegalHold': False,
                     'Retention': 'GOVERNANCE', 'RetainUntil': until})
    return {'started_at': (now-dt.timedelta(seconds=1)).isoformat(), 'finished_at': now.isoformat(),
            'verification': {'group': group, 'status': 'PASS_EXACT_NATIVE_READONLY', 'copies': rows}}
