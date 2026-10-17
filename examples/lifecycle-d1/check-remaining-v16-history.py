"""Validate immutable v16 evidence against its own frozen source, with zero I/O."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tempfile
import zipfile

ROOT=Path(__file__).resolve().parents[2]
DATA=Path('docs/acceptance/evidence/issue307-d1/remaining-native01')
SOURCE='de5f7dd2683cc5b5ed3a68528364d6fe59836f10'
ANCHOR='7278e53145ad418c15dd8e52610ad3b3793c3fd6'

def require(ok,message):
    if not ok: raise ValueError(message)

def check():
    raw=subprocess.check_output(['git','show',ANCHOR+':'+(DATA/'evidence-index.json').as_posix()],cwd=ROOT)
    require(raw==(ROOT/DATA/'evidence-index.json').read_bytes(),'v16 index changed')
    entries=json.loads(raw)['files']
    require(len(entries)==76,'v16 membership changed')
    for path,digest in entries.items():
        name=PurePosixPath(path)
        require(not name.is_absolute() and '..' not in name.parts,'foreign evidence path')
        require(hashlib.sha256((ROOT/DATA/path).read_bytes()).hexdigest()==digest,'v16 payload changed: '+path)
    source=json.loads((ROOT/DATA/'source-freeze.json').read_text())
    require(source['source_commit']==SOURCE and len(source['files'])==768,'v16 source identity changed')
    archive=subprocess.check_output(['git','archive','--format=zip',SOURCE],cwd=ROOT)
    with tempfile.TemporaryDirectory(prefix='dpp-v16-history-') as directory:
        target=Path(directory)
        with zipfile.ZipFile(io.BytesIO(archive)) as files:
            for path,hashes in source['files'].items():
                name=PurePosixPath(path)
                require(not name.is_absolute() and '..' not in name.parts,'foreign source path')
                content=files.read(path)
                require(hashlib.sha256(content.replace(b'\r\n',b'\n')).hexdigest()==hashes['lf_sha256'],'v16 frozen source differs: '+path)
                destination=target/path;destination.parent.mkdir(parents=True,exist_ok=True);destination.write_bytes(content)
        shutil.copytree(ROOT/DATA,target/DATA,dirs_exist_ok=True)
        # The original audit also verifies the three public native IAM payloads.
        for run in ('808a6960-6c36-4200-ae98-f18a68472fe5','754ea8f0-6ffc-4cdc-ac28-40fe8b590a1b','dfda43b4-4419-433d-959b-c617556953fd'):
            path=Path('examples/lifecycle-d1/results')/run/'delete-policy.json'
            (target/path).parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(ROOT/path,target/path)
        spec=importlib.util.spec_from_file_location('v16_audit',ROOT/DATA/'audit.py')
        audit=importlib.util.module_from_spec(spec);spec.loader.exec_module(audit)
        audit.ROOT=target;audit.HERE=target/DATA
        original=subprocess.check_output
        def git_read(args,**kwargs):
            require(args[:2] in (['git','ls-files'],['git','show']),'unexpected historical audit command')
            return original(args,cwd=ROOT)
        audit.subprocess=type('ReadOnlyGit',(),{'check_output':staticmethod(git_read)})
        result=audit.audit()
    print(json.dumps({'status':'PASS_V16_HISTORICAL_OFFLINE','source':SOURCE,'payloads':76,'files':768,'original_audit':result,'current_source_replaced':False,'provider_io':0,'delete':0}))

if __name__=='__main__':check()
