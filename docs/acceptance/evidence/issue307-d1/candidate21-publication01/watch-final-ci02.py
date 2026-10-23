"""Direct-run readback after the separately retained workflow-lookup timeout."""
import datetime as dt
import json
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[5]
OUT = Path(__file__).parent / 'ci-watch02'
HEAD = 'f4f27c789f53b6c45910f55f83bee6cabfdee4f6'


def main():
    OUT.mkdir(exist_ok=False)
    for sequence in range(60):
        result = subprocess.run(['gh', 'run', 'view', '39773563527', '--repo',
            'qq550723504/data-product-platform', '--json',
            'databaseId,headSha,status,conclusion,event,jobs'],
            cwd=ROOT, capture_output=True, timeout=40)
        for label in ('stdout', 'stderr'):
            with (OUT / f'{sequence:03d}-{label}.txt').open('xb') as stream:
                stream.write(getattr(result, label))
        if result.returncode:
            raise ValueError('Fresh CI read transport/process failure; preserve observation')
        row = json.loads(result.stdout)
        if row['headSha'] != HEAD or row['event'] != 'pull_request':
            raise ValueError('Fresh CI provenance differs')
        if row['status'] == 'completed':
            summary = dict(at=dt.datetime.now(dt.timezone.utc).isoformat(),
                head=HEAD, run=row['databaseId'], conclusion=row['conclusion'],
                jobs=len(row['jobs']), success=sum(x['conclusion']=='success' for x in row['jobs']))
            with (OUT / 'completed.json').open('xb') as stream:
                stream.write((json.dumps(summary, indent=2)+'\n').encode())
            print(json.dumps(summary), flush=True)
            if row['conclusion'] != 'success':
                raise ValueError('Exact CI did not succeed')
            return
        time.sleep(30)
    raise ValueError('Bounded fresh CI observation ended')


if __name__ == '__main__':
    main()
