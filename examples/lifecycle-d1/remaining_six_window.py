"""Current six-target window contract, separate from archived seven-target data."""
import remaining_window as base

CASE_MODES = dict(base.CASE_MODES, **{'business-races': {
    'business': 'remaining-resume-business'}})
compiled = base.compiled
instant = base.instant
project_case = base.project_case


def case_profile(template, heads, token, epoch):
    return base.case_profile(template, heads, token, epoch, expected_heads=5)


def validate_case(profile, window):
    return base.validate_case(profile, window, expected_heads=5)


def validate_packet(packet, owner_root=None):
    return base.validate_packet(packet, owner_root, case_modes=CASE_MODES,
                                expected_recoveries=6, expected_heads=5, expected_case_count=6)
