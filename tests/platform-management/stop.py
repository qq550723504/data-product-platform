"""Recover shutdown for one interrupted managed run; never adopt legacy fixtures."""
import argparse
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tests'))
from acceptance_lifecycle import AcceptanceRun


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run', required=True, help='Exact directory name below this checkout .data')
    args = parser.parse_args()
    if not re.fullmatch('[a-z0-9][a-z0-9-]{0,100}', args.run):
        raise RuntimeError('Exact local run directory required')
    scope = AcceptanceRun.reopen(ROOT, ROOT / '.data' / args.run)
    try:
        print(json.dumps(scope.stop()))
    finally:
        scope.close()


if __name__ == '__main__':
    try:
        main()
    except Exception:
        print('FAIL shutdown ownership or state unverified; all data/evidence retained', file=sys.stderr)
        sys.exit(1)
