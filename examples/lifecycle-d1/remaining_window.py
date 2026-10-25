"""Current fixture window contract; stdlib only, no I/O to runtime/providers.

Go's compiled scope is authoritative. Profiles are generated from that one END;
public projections bind exact private bytes without publishing credentials.
Historical packets remain evidence, never current executable candidates.
"""
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import uuid

ROOT = Path(__file__).resolve().parents[2]
CASE_MODES = {'business-races': {'renew': 'remaining-freeze-policy', 'business': 'remaining-business'},
              'late-hold': {'late': 'remaining-late', 'close-H1': 'remaining-close-H1'},
              'mixed': {'mixed': 'remaining-mixed', 'mixed-b': 'remaining-mixed-b', 'mixed-c': 'remaining-mixed-c'}}

def need(ok, message):
    if not ok: raise ValueError(message)

def instant(text):
    need(isinstance(text, str), 'window timestamp missing')
    value = dt.datetime.fromisoformat(text.replace('Z', '+00:00'))
    need(value.tzinfo is not None and value.utcoffset() == dt.timedelta(), 'window must be UTC')
    need(value.isoformat(timespec='seconds').replace('+00:00', 'Z') == text, 'window must be canonical UTC seconds')
    return value

def compiled():
    source = (ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.go').read_text(encoding='utf-8')
    values = {}
    for name in ('Execution', 'Profile', 'NotBefore', 'NotAfter'):
        found = re.findall(r'^const Remaining'+name+r' = "([^"]+)"$', source, re.M)
        need(len(found) == 1, 'compiled window constant missing or duplicated')
        values[name] = found[0]
    need(str(uuid.UUID(values['Execution'])) == values['Execution'], 'execution must be UUID')
    start, end = instant(values['NotBefore']), instant(values['NotAfter'])
    need(end-start == dt.timedelta(minutes=40), 'fixed window must be forty minutes')
    values['RecoveryEnd'] = (end+dt.timedelta(hours=2)).isoformat().replace('+00:00', 'Z')
    return values

def case_profile(template, heads, token, epoch, *, expected_heads=6):
    w = compiled(); result = copy.deepcopy(template)
    result.update(Batch=w['Execution'], CandidateUseUntil=w['NotAfter'], ExpectedPolicyHeads=heads)
    result['Base'].update(StartedAt=w['NotBefore'], ExpiresAt=w['NotAfter'], Token=token, Epoch=epoch)
    result['Base'].pop('RootAccess', None); result['Base'].pop('RootSecret', None)
    validate_case(result, w, expected_heads=expected_heads)
    return result

def validate_case(profile, w, *, expected_heads=6):
    need(profile['Batch'] == w['Execution'], 'case execution differs')
    need(profile['Base']['StartedAt'] == w['NotBefore'] and profile['Base']['ExpiresAt'] == w['NotAfter'], 'case Base window differs')
    need(profile['CandidateUseUntil'] == w['NotAfter'], 'CandidateUseUntil must equal window END')
    if profile.get('Group') == 'business-races':
        need(len(profile['ExpectedPolicyHeads']) == expected_heads, 'expected policy heads membership differs')

def project_case(profile, group, stage, config_sha256):
    return {'group': group, 'stage': stage, 'config_sha256': config_sha256,
            'Batch': profile['Batch'], 'CandidateUseUntil': profile['CandidateUseUntil'],
            'Base': {key: profile['Base'][key] for key in ('StartedAt', 'ExpiresAt')}}

def validate_packet(packet, owner_root=None, *, case_modes=CASE_MODES, expected_recoveries=7, expected_heads=6, expected_case_count=7):
    """Semantic gate before Docker inspection/start; raw hashes cannot waive it."""
    w = compiled(); c = json.loads((packet/'candidate.json').read_bytes())
    for key in ('Execution', 'Profile', 'NotBefore', 'NotAfter'):
        need(c[key] == w[key], 'candidate/compiled '+key+' differs')
    path = packet/'profile-windows.json'; raw = path.read_bytes(); receipt = json.loads(raw)
    need(hashlib.sha256(raw).hexdigest() == c['ProfileWindowsSHA256'], 'profile window receipt anchor differs')
    cases, recoveries = receipt['cases'], receipt['recoveries']
    expected_cases = {(g, stage) for g, modes in case_modes.items() for stage in modes}
    recovery_members = {(g['Group'], stage) for g in c['Groups'] for stage in g['Helpers'] if stage.startswith('recover-')}
    need(len(cases) == expected_case_count and {(r['group'], r['stage']) for r in cases} == expected_cases, 'case window membership differs')
    need(len(recoveries) == expected_recoveries and len(recovery_members) == expected_recoveries and {(r['group'], r['stage']) for r in recoveries} == recovery_members, 'recovery window membership differs')
    groups = {g['Group']: g for g in c['Groups']}
    need(set(groups) == set(case_modes) and len(c['Groups']) == 3, 'window group membership differs')
    for row in cases:
        group, stage = row['group'], row['stage']; g = groups[group]
        need(set(g['CaseHelpers']) == set(case_modes[group]) == set(g['CaseConfigSHA256']), 'case binding membership differs')
        need(row['config_sha256'] == g['CaseConfigSHA256'][stage], 'case window config binding differs')
        validate_case(dict(row, Group=group, ExpectedPolicyHeads=g['PolicyHeads']), w, expected_heads=expected_heads)
        if owner_root is not None:
            path = owner_root/group/('case-'+stage)/'remaining.json'; actual_raw = path.read_bytes(); actual = json.loads(actual_raw)
            need(hashlib.sha256(actual_raw).hexdigest() == row['config_sha256'], 'actual case config differs')
            validate_case(actual, w, expected_heads=expected_heads)
            need(actual['ExpectedPolicyHeads'] == g['PolicyHeads'], 'actual expected policy heads differ')
            need(actual['Group'] == group and project_case(actual, group, stage, row['config_sha256']) == row, 'actual case projection differs')
    for row in recoveries:
        need(row['ValidUntil'] == w['RecoveryEnd'], 'recovery deadline must equal window END plus two hours')
        if owner_root is not None:
            path = owner_root/row['group']/row['stage']/'recovery.json'; raw = path.read_bytes(); actual = json.loads(raw)
            need(hashlib.sha256(raw).hexdigest() == row['config_sha256'] and actual['ValidUntil'] == row['ValidUntil'], 'actual recovery config/deadline differs')
    return {'status': 'PASS_CURRENT_CROSS_FIELD_WINDOWS', 'execution': w['Execution'],
            'candidate_use_until': w['NotAfter'], 'recovery_until': w['RecoveryEnd'],
            'cases': len(cases), 'recoveries': len(recoveries), 'provider_io': 0}
