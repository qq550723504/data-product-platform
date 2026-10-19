"""Current six-target window contract, separate from archived seven-target data."""
import remaining_window as base
import hashlib
import json

POLICY_CHECKPOINT_SHA256 = 'aab0e61530bee63429a15ce635880a6d1db286cb744767f7d9fdb50082313df8'


def policy_checkpoint():
    raw = (base.ROOT/'apps/platform/internal/lifecycle/pilot/remaining_policy_heads.json').read_bytes()
    base.need(hashlib.sha256(raw).hexdigest() == POLICY_CHECKPOINT_SHA256, 'actual current policy checkpoint bytes differ')
    return json.loads(raw)


def policy_heads(group):
    return policy_checkpoint()['heads'].copy() if group == 'business-races' else {}


def validate_policy_heads(group, actual):
    base.need(actual == policy_heads(group), 'current policy checkpoint identity or membership differs')

CASE_MODES = dict(base.CASE_MODES, **{'business-races': {
    'business': 'remaining-resume-business'}})
compiled = base.compiled
instant = base.instant
project_case = base.project_case


def case_profile(template, heads, token, epoch):
    validate_policy_heads(template['Group'], heads)
    return base.case_profile(template, heads, token, epoch, expected_heads=5)


def validate_case(profile, window):
    validate_policy_heads(profile['Group'], profile['ExpectedPolicyHeads'])
    return base.validate_case(profile, window, expected_heads=5)


def validate_packet(packet, owner_root=None):
    candidate = json.loads((packet/'candidate.json').read_bytes())
    base.need(candidate['PolicyCheckpointSHA256'] == POLICY_CHECKPOINT_SHA256, 'candidate policy checkpoint differs')
    for group in candidate['Groups']:
        validate_policy_heads(group['Group'], group['PolicyHeads'])
    result = base.validate_packet(packet, owner_root, case_modes=CASE_MODES,
                                expected_recoveries=6, expected_heads=5, expected_case_count=6)
    result['current_policy_checkpoint_sha256'] = POLICY_CHECKPOINT_SHA256
    return result
