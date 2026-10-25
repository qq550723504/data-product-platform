"""Write acyclic, file-only final preparation bindings after independent review.

This evidence composer has no Docker, network, permission or signing operation.
The frozen complete entry performs the subsequent typed audit.
"""
import hashlib
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[5]
BASE = ROOT / 'docs/acceptance/evidence/issue307-d1'
HERE = BASE / 'candidate22-planning01'
PACKET = BASE / 'remaining-candidate22'
REVIEW = BASE / 'candidate22-artifacts-review01'


def load(path):
    return json.loads(path.read_bytes())


def require(condition, message):
    if not condition:
        raise ValueError(message)


def meta(path):
    raw = path.read_bytes()
    return {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}


def save(path, value):
    with path.open('xb') as stream:
        stream.write((json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8'))


def main():
    candidate = load(PACKET / 'candidate.json')
    report_path = REVIEW / 'independent-final-review01.json'
    report = load(report_path)
    require(report['status'] == 'PASS_BOUNDED_C22_FROZEN_NONDELETE_PREPARATION_REVIEW'
            and report['remaining_blockers'] == []
            and report['execution'] == candidate['Execution']
            and report['runtime_source'] == candidate['Source']
            and report['host_source'] == load(PACKET/'source-freeze.json')['host']['source'], 'independent packet review incomplete')
    host_index = HERE / 'candidate22-host-bindings01.json'
    inputs = {ROOT / name for name in load(host_index)['files']}
    inputs.add(host_index)
    for directory in (PACKET, REVIEW, BASE / 'candidate22-source-review02', BASE / 'remaining-candidate22-readonly01'):
        for path in directory.rglob('*'):
            require(not path.is_symlink() and not path.is_junction(), 'linked final evidence')
            if path.is_file():
                inputs.add(path)
    for name in ('issue307-d1-candidate22-confirmation.md', 'issue307-d1-v32-window-runbook.md'):
        inputs.add(ROOT / 'docs/acceptance' / name)
    inputs.add(ROOT/'docs/acceptance/issue307-d1-v32-preparation-contract03.md')
    inputs.add(HERE / 'final-cold-inventory01.json')
    inputs.add(HERE / 'retained-backups75-readback01.json')
    inputs.add(HERE / 'authority-schedule-absence01.json')
    index_path = HERE / 'final-static-reference-index01.json'
    index = {'status': 'C22_FINAL_STATIC_PREPARATION_REFERENCE_INDEX',
             'execution': candidate['Execution'], 'count': len(inputs),
             'files': {path.relative_to(ROOT).as_posix(): meta(path) for path in sorted(inputs)},
             'authorization': 'NOT_RECEIVED', 'schedule': 'NOT_CREATED',
             'execution_authority': False,
             'scope': 'Saved immutable preparation, native receipts, packet and independent reviews; no future authorization or schedule is invented. Current matrix and post-publication CI receipts are separate append-only observations.'}
    save(index_path, index)
    plan = load(PACKET / 'preparation-evidence-audit-plan.json')
    for identity, path, member_root in (
            ('source-reviewed-external-artifacts', BASE/'candidate22-source-review02/reviewed-artifacts01.json', '.'),
            ('frozen-host-bindings', host_index, '.'),
            ('packet-independent-review', REVIEW / 'evidence-index.json', REVIEW.relative_to(ROOT).as_posix()),
            ('independent-reviewed-external-artifacts', REVIEW / 'reviewed-artifacts01.json', '.'),
            ('final-static-references', index_path, '.')):
        document = load(path)
        plan['indexes'].append({'id': identity, 'index': path.relative_to(ROOT).as_posix(),
                                'sha256': meta(path)['sha256'], 'format': 'files-map-v1',
                                'member_root': member_root, 'expected_count': document['count'],
                                'expected_status': document['status']})
    plan_path = HERE / 'final-static-evidence-audit-plan01.json'
    save(plan_path, plan)
    members = sorted(inputs | {index_path, plan_path})
    manifest_path = HERE / 'approval-reference-manifest01.json'
    save(manifest_path, {
        'status': 'FROZEN_C22_V32_APPROVAL_REFERENCE_MANIFEST_NOT_AUTHORIZED',
        'profile': candidate['Profile'], 'execution': candidate['Execution'],
        'candidate_sha256': meta(PACKET / 'candidate.json')['sha256'],
        'bindings_sha256': meta(PACKET / 'bindings.json')['sha256'],
        'referenced_template_sha256': meta(PACKET / 'confirmation-template.txt')['sha256'],
        'not_before': candidate['NotBefore'], 'not_after': candidate['NotAfter'],
        'max_per_target': 1, 'max_total': 6, 'survivors': 11,
        'unknown_resend': False, 'cleanup': 0, 'hold_release': 0, 'bypass': 0,
        'source': candidate['Source'],
        'final_static_audit_plan': {'path': plan_path.relative_to(ROOT).as_posix(), **meta(plan_path)},
        'count': len(members),
        'approved_reference_artifacts': [
            {'path': path.relative_to(ROOT).as_posix(), **meta(path)} for path in members],
        'authorization': 'NOT_RECEIVED', 'schedule': 'NOT_CREATED',
        'execution_authority': False, 'old_authorization_reused': False,
        'instructions': 'Future actual human message and machine template remain separate. Pin this manifest itself in the later actual authorization. The manifest excludes itself, future authorization/schedule, mutable matrix and later publication receipts to avoid reference cycles.'})
    print(json.dumps({'static_members': len(inputs), 'audit_indexes': len(plan['indexes']),
                      'audit_member_references': sum(x['expected_count'] for x in plan['indexes']),
                      'manifest_members': len(members), 'plan': meta(plan_path),
                      'manifest': meta(manifest_path), 'native_delete': 0,
                      'execution_authority': False}))


if __name__ == '__main__':
    main()
