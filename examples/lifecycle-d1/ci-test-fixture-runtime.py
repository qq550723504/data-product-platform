"""Rebuild the separately pinned fixture R with native Git/Go in PR CI."""
import os
from pathlib import Path
import subprocess
import tempfile
import io
import zipfile
import fixture_artifacts as f
import remaining_six_window as window

def main():
    root=Path(__file__).resolve().parents[2]
    raw=f.load(root/f.SPEC)
    # Local worktrees may already share the complete immutable commit. Never make
    # their shared repository shallow for a CI convenience.
    present=subprocess.run(['git','cat-file','-e',raw['source']+'^'],cwd=root,capture_output=True)
    if present.returncode:
        subprocess.run(['git','-c','core.longpaths=true','fetch','--no-tags','origin',raw['source']],cwd=root,check=True)
    w=window.compiled()
    spec=f.specification(root,execution=w['Execution'],profile=w['Profile'],start=w['NotBefore'],end=w['NotAfter'])
    with tempfile.TemporaryDirectory(prefix='d1-fixture-R-') as directory:
        control=Path(directory)
        archive_bytes=f.runtime_archive(root,spec['source'])
        print('R_ARCHIVE expected='+spec['archive_sha256']+' observed='+f.digest(archive_bytes)+' bytes='+str(len(archive_bytes)),flush=True)
        (control/'approved-runtime-source.zip').write_bytes(archive_bytes)
        files=f.verify_archive(root,control,spec)
        for name,content in files.items():
            path=control/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(content)
        # Existing Go tests read immutable historical receipts by repository path.
        # Testdata is separately taken from R; it is never a runtime build member.
        test_roots=['docs/acceptance/evidence/issue307-d1/remaining-candidate'+n+'/' for n in ['07','09','10','11','12']]+['docs/acceptance/evidence/issue307-d1/delete-confirmed-v17-window01/']
        data=f.git(root,'archive','--format=zip',spec['source'],*[p.rstrip('/') for p in test_roots])
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            for entry in archive.infolist():
                if entry.is_dir():continue
                path=control/entry.filename
                f.need(path.resolve().is_relative_to(control.resolve()) and entry.filename.startswith(tuple(test_roots)), 'foreign historical testdata member')
                path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(archive.read(entry))
        env=os.environ.copy()
        for key in ['POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT']:
            env.pop(key,None)
        env.update(GOWORK='off',GOFLAGS='',CGO_ENABLED='0')
        base=control/'apps/platform'
        subprocess.run(['go','test','./internal/lifecycle/pilot','-run','TestRemaining(FixtureSchemaSingleContract|Retained|DeletePolicy)','-count=1'],cwd=base,env=env,check=True)
        subprocess.run(['go','test','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-remaining','./cmd/lifecycle-fixture-remaining-iam','./cmd/lifecycle-fixture-readonly-check','-count=1'],cwd=base,env=env,check=True)
    print('PASS_SEPARATE_R_ARCHIVE_BUILD_AND_OFFLINE_CONTRACTS; no source/provider I/O')

if __name__=="__main__":
    try:main()
    except Exception as error:
        # Fixed CI-only inputs have no source/provider secrets or credentials.
        detail=(type(error).__name__+": "+str(error)).replace("%","%25").replace("\r","%0D").replace("\n","%0A")
        print("::error title=Separate fixture archive CI::"+detail,flush=True)
        raise
