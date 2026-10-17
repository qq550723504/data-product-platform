"""Run the unchanged v17 strict checker/tests against their frozen runtime.

Current runtime is separately checked by check-remaining-candidate09.py. This
adapter never permits source drift inside either generation and never touches
Docker, provider credentials or the retained environment.
"""
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = 'f3b5de45857add3f852bbac0348e8c30ea105ade'
TOOLS = 'b4d0556ad424e15896577a069bcbd1b236647a07'
PACKET = Path('docs/acceptance/evidence/issue307-d1/remaining-candidate07')

def need(ok, message):
    if not ok: raise ValueError(message)
def digest(raw): return hashlib.sha256(raw).hexdigest()
def git(*args): return subprocess.check_output(['git', *args], cwd=ROOT)
def path_under(root, name):
    p = PurePosixPath(name)
    need(not p.is_absolute() and '..' not in p.parts and '\\' not in name, 'unsafe historical path')
    target = root/name
    need(target.resolve().is_relative_to(root.resolve()), 'historical path escaped root')
    return target

def check():
    index_path = PACKET/'evidence-index.json'
    index_raw = git('show', '9a41e13f1ebe01ae90b36f53c491392c3c4a72ad:'+index_path.as_posix())
    need((ROOT/index_path).read_bytes() == index_raw, 'historical index changed')
    index = json.loads(index_raw); records = dict(index['files'], **index['references'])
    need(len(records) == 940, 'historical raw membership changed')
    source = json.loads((ROOT/PACKET/'source-freeze.json').read_bytes())
    need(source['source_commit'] == SOURCE and len(source['files']) == 812, 'historical source identity changed')
    archive = git('archive', '--format=zip', SOURCE)
    tools = ['check-remaining-candidate07-freeze.py', 'test_remaining_candidate07.py', 'test_remaining_candidate07_freeze.py']
    with tempfile.TemporaryDirectory(prefix='dpp-v17-frozen-history-') as directory:
        target = Path(directory)
        with zipfile.ZipFile(io.BytesIO(archive)) as files:
            for name, hashes in source['files'].items():
                raw = files.read(name)
                need(digest(raw.replace(b'\r\n',b'\n')) == hashes['lf_sha256'], 'historical source content changed')
                destination = path_under(target, name); destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(raw)
        for name, expected in records.items():
            raw = path_under(ROOT, name).read_bytes()
            need(digest(raw) == expected, 'historical evidence content changed')
            destination = path_under(target,name); destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(raw)
        destination = target/index_path; destination.parent.mkdir(parents=True, exist_ok=True); destination.write_bytes(index_raw)
        for name in tools:
            relative = 'examples/lifecycle-d1/'+name
            pinned = git('show', TOOLS+':'+relative)
            need((ROOT/relative).read_bytes().replace(b'\r\n',b'\n') == pinned.replace(b'\r\n',b'\n'), 'historical checker/test was changed')
            (target/relative).write_bytes(pinned)
        # Preserve the exact already-approved gofmt correction; do not admit any
        # other current-source substitution into this historical runtime.
        relative = 'apps/platform/cmd/lifecycle-fixture-remaining-iam/main_test.go'
        formatted = git('show',TOOLS+':'+relative)
        need(digest(formatted.replace(b'\r\n',b'\n')) == 'f823e77aff612ea1b542754c68db2e6ab68aaaa28bf6c517e14754980b0b4fe5', 'reviewed historical test differs')
        (target/relative).write_bytes(formatted)
        env = os.environ.copy()
        env['GIT_DIR'] = git('rev-parse','--absolute-git-dir').decode().strip()
        env['GIT_WORK_TREE'] = str(target)
        # All pinned commands are read-only Git show/archive/cat-file/gofmt and
        # isolated unit tests. Their index is the real checkout's exact raw blobs.
        completed = subprocess.run([sys.executable,'-B',str(target/'examples/lifecycle-d1/check-remaining-candidate07-freeze.py')],
                                   cwd=target,env=env,capture_output=True)
        if completed.returncode:
            sys.stderr.buffer.write(completed.stderr); raise ValueError('unchanged v17 strict historical audit failed')
        strict = json.loads(completed.stdout)
        for test in tools[1:]:
            for optimized in (False,True):
                command = [sys.executable] + (['-O'] if optimized else []) + ['-B',str(target/'examples/lifecycle-d1'/test),'-v']
                result = subprocess.run(command,cwd=target,env=env,capture_output=True)
                sys.stdout.buffer.write(result.stdout); sys.stderr.buffer.write(result.stderr)
                need(result.returncode == 0, 'unchanged historical rejection tests failed')
    return {'status':'PASS_V17_HISTORICAL_STRICT','historical_runtime':SOURCE,
            'historical_tools_anchor':TOOLS,'source_files':812,'raw_evidence':940,
            'historical_strict_result':strict,'current_runtime_not_substituted':True,
            'current_runtime_check':'check-remaining-candidate09.py','provider_io':0,'container_starts':0,'delete':0}

if __name__ == '__main__': print(json.dumps(check()))
