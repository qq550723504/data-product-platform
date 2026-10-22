"""Audit the unchanged executable candidate and its exact retained source snapshot.

Only the one reviewed test-only gofmt result may differ in the current tree.
All 940 raw receipts must match both the anchored index and the Git index blobs.
No Docker, SQL, provider, network, or private credential access.
"""
import hashlib
import importlib.util
import io
import json
from pathlib import Path, PurePosixPath
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
PACKET = Path('docs/acceptance/evidence/issue307-d1/remaining-candidate07')
SOURCE = 'f3b5de45857add3f852bbac0348e8c30ea105ade'
ANCHOR = '9a41e13f1ebe01ae90b36f53c491392c3c4a72ad'
FORMATTED_TEST = 'apps/platform/cmd/lifecycle-fixture-remaining-iam/main_test.go'
FORMATTED_TEST_SHA = 'f823e77aff612ea1b542754c68db2e6ab68aaaa28bf6c517e14754980b0b4fe5'


def require(ok, message):
    if not ok:
        raise ValueError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def safe_path(root, name):
    path = PurePosixPath(name)
    require(not path.is_absolute() and '..' not in path.parts and '\\' not in name,
            'unsafe evidence/source path')
    target = root / name
    require(target.resolve().is_relative_to(root.resolve()), 'path escaped root')
    return target


def anchored_bytes(actual, anchored, name):
    require(actual == anchored, 'frozen anchor changed: ' + name)


def verify_raw(root, records, git_blobs):
    require(set(records) == set(git_blobs), 'Git raw membership differs')
    for name, digest in records.items():
        path = safe_path(root, name)
        require(path.is_file() and sha(path.read_bytes()) == digest,
                'raw evidence changed: ' + name)
        require(sha(git_blobs[name]) == digest, 'Git raw bytes changed: ' + name)


def verify_current_source(root, files):
    require(FORMATTED_TEST in files, 'reviewed test absent from source manifest')
    for name, hashes in files.items():
        path = safe_path(root, name)
        expected = FORMATTED_TEST_SHA if name == FORMATTED_TEST else hashes['lf_sha256']
        require(path.is_file() and sha(path.read_bytes().replace(b'\r\n', b'\n')) == expected,
                'current source differs from frozen runtime/reviewed test: ' + name)


def index_blobs(root, names):
    names = sorted(names)
    expressions = [':' + name for name in names]
    data = subprocess.check_output(['git', 'cat-file', '--batch'], cwd=root,
                                   input=('\n'.join(expressions) + '\n').encode())
    offset = 0
    result = {}
    for name in names:
        end = data.index(b'\n', offset)
        header = data[offset:end].split()
        require(len(header) == 3 and header[1] == b'blob', 'missing Git index blob: ' + name)
        size = int(header[2]); offset = end + 1
        result[name] = data[offset:offset + size]
        offset += size
        require(data[offset:offset + 1] == b'\n', 'malformed Git batch result')
        offset += 1
    require(offset == len(data), 'unexpected Git batch result')
    return result


def check(root=ROOT):
    index_path = PACKET / 'evidence-index.json'
    anchored_index = subprocess.check_output(['git', 'show', ANCHOR + ':' + index_path.as_posix()], cwd=root)
    anchored_bytes((root / index_path).read_bytes(), anchored_index, 'evidence-index.json')
    candidate_path = PACKET / 'candidate.json'
    anchored_candidate = subprocess.check_output(['git', 'show', ANCHOR + ':' + candidate_path.as_posix()], cwd=root)
    anchored_bytes((root / candidate_path).read_bytes(), anchored_candidate, 'candidate.json')
    index = json.loads(anchored_index)
    require(not set(index['files']).intersection(index['references']), 'duplicate raw membership')
    records = dict(index['files'], **index['references'])
    require(len(records) == 940, 'frozen raw membership differs')
    verify_raw(root, records, index_blobs(root, records))
    source = json.loads((root / PACKET / 'source-freeze.json').read_bytes())
    require(source['source_commit'] == SOURCE and len(source['files']) == 812,
            'frozen source identity differs')
    verify_current_source(root, source['files'])
    archive = subprocess.check_output(['git', 'archive', '--format=zip', SOURCE], cwd=root)
    with tempfile.TemporaryDirectory(prefix='dpp-candidate07-freeze-') as directory:
        target = Path(directory)
        with zipfile.ZipFile(io.BytesIO(archive)) as files:
            for name, hashes in source['files'].items():
                content = files.read(name)
                require(sha(content.replace(b'\r\n', b'\n')) == hashes['lf_sha256'],
                        'frozen source differs: ' + name)
                destination = safe_path(target, name)
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(content)
        for name in [*records, index_path.as_posix()]:
            destination = safe_path(target, name)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes((root / name).read_bytes())
        script = target / 'examples/lifecycle-d1/check-remaining-candidate07.py'
        spec = importlib.util.spec_from_file_location('candidate07_frozen_audit', script)
        audit = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(audit)
        result = audit.check(target)
    return dict(result, status='PASS_FROZEN_CANDIDATE07_OFFLINE', frozen_source=SOURCE,
                original_audit='UNCHANGED_F3_CHECKER', current_runtime_source_drift=0,
                current_test_gofmt_only=FORMATTED_TEST, git_raw_blobs_checked=940,
                candidate_bytes_changed=False, provider_io=0, db_io=0, container_starts=0)


if __name__ == '__main__':
    print(json.dumps(check()))
