"""Authorization receipt readback through the frozen host check, read-only."""
import datetime
import importlib.util
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[6]
OUT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
spec = importlib.util.spec_from_file_location('v26_authorized_host_check', ROOT / 'examples/lifecycle-d1/window_six_support.py')
host = importlib.util.module_from_spec(spec)
spec.loader.exec_module(host)
trace, blocked = [], []
original = subprocess.Popen


class ReadOnlyPopen(original):
    def __init__(self, args, *a, **kw):
        argv = [str(x) for x in args]
        allowed = not kw.get('shell') and argv[0] == 'git' and argv[1] in ('cat-file', 'diff', 'ls-files', 'rev-parse', 'show')
        if not kw.get('shell') and argv[0] == 'docker':
            allowed = (argv[1] == 'inspect' or argv[1:3] in (['volume', 'inspect'], ['network', 'inspect'])
                       or argv[1] == 'ps' or (argv[1] == 'cp' and argv[-1] == '-'
                       and (argv[-2].endswith(':/run/budget') or argv[-2].endswith(':/run/budget/.'))))
        if not allowed:
            blocked.append(argv)
            raise RuntimeError('Authorization readback rejected non-read-only process')
        trace.append(argv)
        super().__init__(args, *a, **kw)


subprocess.Popen = ReadOnlyPopen
failure = None
try:
    result = host.Host().check()
    result.update(observed_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  read_only_processes=len(trace), unapproved_processes=blocked,
                  permission_enable=0, proof_signing=0, source_start_or_exec=0,
                  schedule='NOT_CREATED', future_window_fresh_preconditions='REQUIRED')
    with (OUT / 'authorized-prewindow-check.json').open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
    print(json.dumps(result))
except BaseException as error:
    failure = {'type': type(error).__name__, 'message': str(error)}
    raise
finally:
    subprocess.Popen = original
    with (OUT / 'process-trace.json').open('x', encoding='utf-8', newline='\n') as stream:
        json.dump({'commands': trace, 'blocked': blocked, 'failure': failure}, stream, indent=2)
        stream.write('\n')
