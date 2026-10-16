"""One fixed audit-core-08 read-only refresh of reviewed v8. No send switches."""
import argparse
import importlib.util
from pathlib import Path

SPEC = importlib.util.spec_from_file_location('readonly_refresh', Path(__file__).with_name('refresh-reference-readonly.py'))
refresh = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(refresh)

if __name__ == '__main__':
    argparse.ArgumentParser(description=__doc__).parse_args()
    refresh.run(8)
