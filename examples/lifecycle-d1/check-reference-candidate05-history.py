"""Check archived candidate05 using its exact Git runtime in a public snapshot.

Reuse candidate04's isolation boundary, with a finite candidate05 binding. The
original checker, all 317 frozen source hashes, 35 payloads and 344 older raw
artifacts stay strict. Current application source is never replaced. No native
operator, private directory, credential, Docker socket or binary is copied.
"""
import argparse
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
DATA='docs/acceptance/evidence/issue307-d1/delete-candidate05-preparation01/'
BASE='docs/acceptance/evidence/issue307-d1/'
ANCHOR='b823eb33f8f3f923b619feda06e0ecc53514dd23'
SOURCE='dbe3fbd666146db6a010a508fa99e6ed558b6fe9'
HISTORY='ce7057b296adde0ef9101c4233ff7af051f455d0'

def require(ok,message):
    if not ok:raise ValueError(message)

def archive(revision,path):
    require((revision,path) in {(ANCHOR,DATA.rstrip('/')),(SOURCE,'apps/platform'),(HISTORY,BASE.rstrip('/'))},'Unsupported candidate05 history archive')
    return subprocess.run(['git','archive','--format=zip',revision,path],cwd=ROOT,capture_output=True,check=True).stdout

def current(path):return (ROOT/path).read_bytes()

def build_snapshot(destination, archives):
    require(not destination.exists(),'New public snapshot required')
    with zipfile.ZipFile(io.BytesIO(archives[(ANCHOR,DATA.rstrip('/'))])) as evidence:
        index=json.loads(evidence.read(DATA+'evidence-index.json'))
        require(len(index['files'])==35,'Frozen candidate05 payload membership changed')
        paths={DATA+'evidence-index.json'}
        for entry in index['files']:
            name=PurePosixPath(entry['path'])
            require(entry['path'].startswith(DATA) and not name.is_absolute() and '..' not in name.parts,'Foreign historical evidence path')
            raw=evidence.read(entry['path'])
            require(len(raw)==entry['bytes'] and hashlib.sha256(raw).hexdigest()==entry['sha256'],'Archived candidate05 index differs')
            paths.add(entry['path'])
        for path in paths:require(current(path)==evidence.read(path),'Current historical candidate05 evidence changed: '+path)
        binding=json.loads(evidence.read(DATA+'source-binding.json'))
    require(binding['source_commit']==SOURCE and len(binding['runtime_source_lf_sha256'])==317,'Frozen candidate05 source membership changed')
    destination.mkdir()
    shutil.copytree(ROOT/BASE,destination/BASE)
    scripts=destination/'examples/lifecycle-d1';scripts.mkdir(parents=True)
    for source in (ROOT/'examples/lifecycle-d1').glob('*.py'):shutil.copyfile(source,scripts/source.name)
    with zipfile.ZipFile(io.BytesIO(archives[(SOURCE,'apps/platform')])) as source:
        for path,digest in binding['runtime_source_lf_sha256'].items():
            name=PurePosixPath(path)
            require(not name.is_absolute() and '..' not in name.parts and name.parts[:2]==('apps','platform'),'Frozen runtime path escaped')
            raw=source.read(path)
            require(hashlib.sha256(raw.replace(b'\r\n',b'\n')).hexdigest()==digest,'Archived candidate05 runtime differs: '+path)
            target=destination/path;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw)

def check():
    pairs=((ANCHOR,DATA.rstrip('/')),(SOURCE,'apps/platform'),(HISTORY,BASE.rstrip('/')))
    archives={pair:archive(*pair) for pair in pairs}
    with tempfile.TemporaryDirectory(prefix='dpp-candidate05-history-') as directory:
        destination=Path(directory)/'public-snapshot'
        build_snapshot(destination,archives)
        spec=importlib.util.spec_from_file_location('strict_candidate05',ROOT/'examples/lifecycle-d1/check-reference-candidate05.py')
        checker=importlib.util.module_from_spec(spec);spec.loader.exec_module(checker)
        require(checker.SOURCE==SOURCE and checker.HISTORY==HISTORY and checker.DATA==DATA,'Current checker scope differs')
        checker.ROOT=destination
        def fixed_archive(revision,path):
            require((revision,path) in {(SOURCE,'apps/platform'),(HISTORY,BASE.rstrip('/'))},'Checker requested foreign history')
            return zipfile.ZipFile(io.BytesIO(archives[(revision,path)]))
        checker.archive=fixed_archive
        checker.check()
    print(json.dumps({'status':'PASS_CANDIDATE05_HISTORICAL_OFFLINE','frozen_source':SOURCE,'current_source_replaced':False,'current_historical_payloads_checked':35,'runtime_hashes_checked':317,'old_raw_checked':344,'provider_io':0,'db_io':0,'delete':0}))

def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    check()

if __name__=='__main__':main()
