"""Fresh exact final HEAD CI and review threads using the fixed GitHub reader."""
import argparse
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[5]
sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
import execution_freeze as f


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    out = args.output.absolute()
    f.need(out.resolve() == out and out.is_relative_to(ROOT / 'docs/acceptance/evidence/issue307-d1'), 'Unaliased public output required')
    out.mkdir(parents=True, exist_ok=False)
    head = subprocess.check_output(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip()
    f.need(not subprocess.check_output(['git', '-c', 'core.longpaths=true', 'diff', '--name-only', 'HEAD'], cwd=ROOT).strip(), 'Final tracked files drift')
    client = f.GitHub(out)
    snapshot = client.snapshot()
    f.ready(snapshot, head)
    f.need(len(snapshot['ci']['jobs']) == 13, 'Necessary workflow membership differs')
    f.save(out / 'snapshot.json', snapshot)
    q = '''query { repository(owner:"qq550723504",name:"data-product-platform") {
      pullRequest(number:332) { headRefOid isDraft state merged
        reviewThreads(first:100) { pageInfo { hasNextPage } nodes { id isResolved } }
      }
      issue(number:307) { state }
    } }'''
    data = client.graph(q)['repository']
    pr = data['pullRequest']
    threads = pr['reviewThreads']
    f.need(pr['headRefOid'] == head and pr['isDraft'] and pr['state'] == 'OPEN' and not pr['merged']
           and data['issue']['state'] == 'OPEN' and not threads['pageInfo']['hasNextPage'], 'PR/thread/Issue observation differs')
    unresolved = [row['id'] for row in threads['nodes'] if not row['isResolved']]
    f.need(not unresolved, 'Unresolved review threads need bounded classification')
    f.save(out / 'review-threads.json', data)
    result = dict(status='PASS_EXACT_FINAL_HEAD_PR_CI_AND_REVIEW_THREADS',
                  at=f.now(), head=head, ci_run=snapshot['ci']['id'], ci_event='pull_request',
                  jobs=13, unresolved_threads=0, Draft=True, Issue307='OPEN',
                  native_delete=0, execution_authority=False, main_CI='NOT_RUN_NOT_MERGED')
    f.save(out / 'result.json', result)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
