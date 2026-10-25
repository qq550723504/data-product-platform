"""Publish the bounded review checkpoint from actual lock receipts; no HEAD write."""
import datetime as dt
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
OUT = HERE / 'pr-checkpoint01'


def save(name, raw):
    with (OUT / name).open('xb') as stream:
        stream.write(raw)


def run(label, args):
    result = subprocess.run(args, cwd=ROOT, capture_output=True, timeout=40)
    save(label + '-stdout.txt', result.stdout)
    save(label + '-stderr.txt', result.stderr)
    save(label + '-process.json', (json.dumps({'argv': args, 'exit': result.returncode,
        'at': dt.datetime.now(dt.timezone.utc).isoformat()}, indent=2) + '\n').encode())
    if result.returncode:
        raise ValueError('PR publication outcome requires retained readback: ' + label)
    return result.stdout


def main():
    OUT.mkdir(exist_ok=False)
    record = json.loads((HERE / 'final-checkpoint01.json').read_bytes())
    b = json.loads((HERE / 'branch-freeze01/execution-binding.json').read_bytes())
    head = record['final_head']
    if subprocess.check_output(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip() != head:
        raise ValueError('Final HEAD differs')
    body = (HERE / 'pr-body-prelock01.md').read_text(encoding='utf-8')
    old = '- Final publication HEAD PR CI and native exact branch lock are separate post-publication gates; neither the source CI nor preparation review substitutes for them. Lock receipts and the final execution-binding must precede any new human authorization.'
    if body.count(old) != 1:
        raise ValueError('Prelock body snapshot differs')
    new = (f"- Final exact HEAD `{head}` PR CI run{record['ci_run']} passed 13/13, event=pull_request; unresolved review threads=0. "
           f"The native exact branch lock is active, rule `{record['rule_id']}`, with administrator enforcement and no fork-sync/force/delete. "
           f"Public/private execution-binding bytes match SHA256 `{record['execution_binding_sha256']}`. "
           "All tracked runtime/source/candidate bytes remain frozen; post-lock observations are appended off HEAD. This is PR CI, not main CI or human authorization.")
    body = body.replace(old, new)
    body_file = OUT / 'body.md'
    save('body.md', body.encode())
    run('edit', ['gh', 'pr', 'edit', '332', '--repo', 'qq550723504/data-product-platform', '--body-file', str(body_file)])
    read = json.loads(run('body-readback', ['gh', 'pr', 'view', '332', '--repo', 'qq550723504/data-product-platform',
        '--json', 'title,body,headRefOid,isDraft,state,mergedAt']))
    if read['body'].encode() != body.encode() or read['headRefOid'] != head or not read['isDraft'] or read['state'] != 'OPEN' or read['mergedAt'] is not None:
        raise ValueError('PR body UTF8/state readback differs')
    comment = f'''Bounded non-delete closure checkpoint for candidate21/v31:

- Exact HEAD `{head}`; PR CI [run {record['ci_run']}](https://github.com/qq550723504/data-product-platform/actions/runs/{record['ci_run']}) passed 13/13. Review threads unresolved=0. This is not main CI.
- Merge contract: exact authorization/credential boundaries, append-only history/provider attempts/cost, consistent frozen contracts, required path/build/CI, and concurrency/idempotency. Current source + frozen preparation independent reviews: remaining blockers=0. Full D1 acceptance and six real sends/H2/M1 outcomes remain tracked by #307 and NOT_RUN.
- Seventeen versions/P1 absence, 47 disabled identities/15 NoSuchUser, 167 paired requests, three independent empty budgets, 996 stopped CIDs and 66 retained backup files passed actual preparation/read-only checks. Candidate20, consumed old budgets, pins and all failures remain.
- Final native rule `{b['branch_lock']['rule_id']}` locks this exact branch; execution-binding SHA256 `{record['execution_binding_sha256']}`. No automatic unlock or further HEAD update. Original preparation snapshots remain immutable; later lock/confirmation receipts are appended off HEAD.
- Proposed window: 2026-10-22 Singapore 22:00-22:40; execution `{record['execution']}`. New delete authority NOT_RECEIVED; scheduling NOT_CREATED; DELETE=0. #332 stays Draft/unmerged, #307 OPEN. Preparation/CI/review/branch lock do not grant deletion authority.
'''
    comment_file = OUT / 'comment.md'
    save('comment.md', comment.encode())
    response = run('comment', ['gh', 'pr', 'comment', '332', '--repo', 'qq550723504/data-product-platform', '--body-file', str(comment_file)])
    url = response.decode().strip()
    comment_id = url.rsplit('issuecomment-', 1)[-1]
    if not comment_id.isdigit():
        raise ValueError('Comment response needs manual readback; do not resend')
    value = json.loads(run('comment-readback', ['gh', 'api', 'repos/qq550723504/data-product-platform/issues/comments/' + comment_id]))
    if value['body'].encode() != comment.encode():
        raise ValueError('Checkpoint UTF8 readback differs')
    save('result.json', (json.dumps(dict(status='PASS_PR_BODY_AND_CHECKPOINT_UTF8_EXACT_READBACK',
        head=head, comment_url=url, PR332='OPEN_DRAFT_UNMERGED', execution_authority=False), indent=2)+'\n').encode())
    print(json.dumps({'status': 'PASS_PR_BODY_AND_CHECKPOINT_UTF8_EXACT_READBACK', 'head': head, 'comment_url': url}))


if __name__ == '__main__':
    main()
