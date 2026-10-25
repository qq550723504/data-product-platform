"""Verify the four actual approval documents through real Git; file-only."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
OUT = HERE / 'publication-bytes-regression01'
NAMES = ['docs/acceptance/.gitattributes', *[
    'docs/acceptance/' + name for name in (
        'issue307-d1-candidate22-confirmation.md',
        'issue307-d1-v32-window-runbook.md',
        'issue307-d1-v32-preparation-contract03.md')]]


def git(root, *args, mode='true'):
    return subprocess.check_output(
        ['git', '-c', 'core.longpaths=true', '-c', 'core.autocrlf=' + mode, *args],
        cwd=root, stderr=subprocess.PIPE)


def main():
    OUT.mkdir(exist_ok=False)
    files = {name: (ROOT / name).read_bytes() for name in NAMES}
    observations = {name: {'bytes': len(raw), 'sha256': hashlib.sha256(raw).hexdigest()}
                    for name, raw in files.items()}
    with tempfile.TemporaryDirectory(prefix='c22-issued-bytes-') as tmp:
        folder = Path(tmp)
        repo = folder / 'source'
        repo.mkdir()
        git(repo, 'init', '-q')
        (repo / '.gitattributes').write_bytes((ROOT / '.gitattributes').read_bytes())
        for name, raw in files.items():
            target = repo / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(raw)
        git(repo, 'add', '.')
        git(repo, '-c', 'user.name=Receipt Test', '-c',
            'user.email=receipt@example.invalid', 'commit', '-qm', 'Actual C22 bytes')
        with zipfile.ZipFile(io.BytesIO(git(repo, 'archive', '--format=zip', 'HEAD'))) as archive:
            for name, raw in files.items():
                if git(repo, 'show', 'HEAD:' + name) != raw or archive.read(name) != raw:
                    raise ValueError('Actual approval blob/archive drift: ' + name)
                observations[name].update(blob_equal=True, archive_equal=True)
        for mode in ('true', 'false'):
            checkout = folder / ('checkout-' + mode)
            git(repo, 'clone', '--no-local', '-q', str(repo), str(checkout), mode=mode)
            for name, raw in files.items():
                if (checkout / name).read_bytes() != raw:
                    raise ValueError('Actual approval checkout drift: ' + name)
                observations[name]['autocrlf_' + mode + '_equal'] = True
    value = dict(status='PASS_ACTUAL_C22_APPROVAL_BYTES_GIT_BLOB_ARCHIVE_CHECKOUT',
                 files=observations, count=4, source_Docker_provider_DELETE=0,
                 authority=False, runtime_source_changed=False)
    with (OUT / 'result.json').open('xb') as stream:
        stream.write((json.dumps(value, indent=2) + '\n').encode())
    print(json.dumps({'status': value['status'], 'files': 4, 'authority': False}))


if __name__ == '__main__':
    main()
