"""Index immutable post-lock receipts; no self-reference or future authority."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
DOCS = ROOT / 'docs/acceptance'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')


def main():
    check = json.loads((HERE / 'final-checkpoint02.json').read_bytes())
    review = json.loads((HERE / 'independent-postlock-review01/independent-final-review01.json').read_bytes())
    if review['status'] != 'PASS_BOUNDED_C21_POSTLOCK_CONFIRMATION_REVIEW' or review['remaining_blockers']:
        raise ValueError('Post-lock independent confirmation incomplete')
    scan = json.loads((HERE / 'publication-secret-scan02.json').read_bytes())
    if scan['status'] != 'PASS_NO_ACTUAL_CREDENTIAL_BYTES' or scan['findings']:
        raise ValueError('Actual credential-byte comparison did not pass')
    if (PRIVATE / 'authorized-execution01').exists() or (HERE.parent / 'delete-confirmed-v31-execution01').exists():
        raise ValueError('Unexpected authority')
    if Path('C:/Users/Henry/.codex/automations/307-d1-candidate21-v31').exists():
        raise ValueError('Unexpected schedule')
    head = subprocess.check_output(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip()
    if head != check['final_head'] or subprocess.check_output(['git', '-c', 'core.longpaths=true', 'diff', '--name-only', 'HEAD'], cwd=ROOT).strip():
        raise ValueError('Locked HEAD/tracked files drift')
    output = HERE / 'publication-evidence-index01.json'
    paths = {p for p in HERE.rglob('*') if p.is_file() and p != output}
    paths.update(DOCS / name for name in (
        'issue307-d1-candidate21-final-confirmation01.md',
        'issue307-d1-candidate21-final-confirmation02.md',
        'issue307-d1-v31-postlock-matrix01.md',
        'issue307-d1-v31-postlock-matrix02.md'))
    files = {}
    for path in sorted(paths):
        if path.is_symlink() or path.is_junction():
            raise ValueError('Linked publication member')
        raw = path.read_bytes()
        files[path.relative_to(ROOT).as_posix()] = {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}
    result = dict(status='C21_POSTLOCK_PUBLICATION_RECEIPT_INDEX_NOT_AUTHORIZED',
        execution=check['execution'], final_head=head, count=len(files), files=files,
        member_root='.', format='files-map-v1', current_proposal=check['final_proposal'],
        current_proposal_sha256=check['final_proposal_sha256'],
        execution_binding_sha256=check['execution_binding_sha256'],
        authorization='NOT_RECEIVED', schedule='NOT_CREATED', native_delete=0,
        full_D1='NOT_ACCEPTED', original_failed_watch_preserved=True,
        scope='Exact saved receipts only; index excludes itself and future authorization/scheduling observations. Original v01 rendering is retained and clarified by v02 without changing runtime/binding/HEAD.')
    with output.open('xb') as stream:
        stream.write((json.dumps(result, indent=2)+'\n').encode())
    print(json.dumps({'status': result['status'], 'members': len(files), 'head': head,
        'sha256': hashlib.sha256(output.read_bytes()).hexdigest(), 'native_delete': 0}))


if __name__ == '__main__':
    main()
