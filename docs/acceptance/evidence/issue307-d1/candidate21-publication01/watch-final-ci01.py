"""Read only the final exact HEAD's CI; no source/provider/branch mutation."""
import datetime as dt
import json
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
OUT = HERE / 'ci-watch01'
HEAD = 'f4f27c789f53b6c45910f55f83bee6cabfdee4f6'


def save(name, raw):
    with (OUT / name).open('xb') as stream:
        stream.write(raw)


def main():
    OUT.mkdir(exist_ok=False)
    last = None
    for sequence in range(110):
        args = ['gh', 'run', 'list', '--repo', 'qq550723504/data-product-platform',
                '--commit', HEAD, '--event', 'pull_request', '--workflow', 'ci.yml',
                '--limit', '20', '--json', 'databaseId,headSha,status,conclusion,url']
        result = subprocess.run(args, cwd=ROOT, capture_output=True, timeout=40)
        save(f'{sequence:03d}-stdout.json', result.stdout)
        save(f'{sequence:03d}-stderr.txt', result.stderr)
        if result.returncode:
            raise ValueError('CI observation transport/process failure; not a CI conclusion')
        rows = json.loads(result.stdout)
        if rows:
            run = max(rows, key=lambda x: x['databaseId'])
            if run['headSha'] != HEAD:
                raise ValueError('CI observation HEAD differs')
            marker = (run['databaseId'], run['status'], run['conclusion'])
            if marker != last:
                print(json.dumps(dict(at=dt.datetime.now(dt.timezone.utc).isoformat(), **run)), flush=True)
                last = marker
            if run['status'] == 'completed':
                save('completed-run.json', (json.dumps(run, indent=2) + '\n').encode())
                if run['conclusion'] != 'success':
                    raise ValueError('Final HEAD CI did not succeed; preserve original logs')
                return
        time.sleep(30)
    raise ValueError('Bounded final CI observation ended without completion')


if __name__ == '__main__':
    main()
