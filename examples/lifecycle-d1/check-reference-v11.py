"""Offline consistency check of fixed v11/audit-core-10; no provider or approval I/O."""
import importlib.util
import json
from pathlib import Path

SPEC = importlib.util.spec_from_file_location('refresh', Path(__file__).with_name('check-reference-refresh.py'))
refresh = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(refresh)
PROPOSAL = refresh.refresh_paths('10')[2]


def check(proposal, root=refresh.ROOT):
    return refresh.check(proposal, root, '10')


if __name__ == '__main__':
    print(json.dumps(check(refresh.load(PROPOSAL)), indent=2))
