"""Rebuild the separately pinned fixture R with native Git/Go in PR CI."""
import os
from pathlib import Path
import subprocess
import tempfile
import fixture_artifacts as f
import remaining_six_window as window

root=Path(__file__).resolve().parents[2]
raw=f.load(root/f.SPEC)
# Fetch only the immutable source and direct parent; no checkout or branch write.
subprocess.run(['git','-c','core.longpaths=true','fetch','--no-tags','--depth=2','origin',raw['source']],cwd=root,check=True)
w=window.compiled()
spec=f.specification(root,execution=w['Execution'],profile=w['Profile'],start=w['NotBefore'],end=w['NotAfter'])
with tempfile.TemporaryDirectory(prefix='d1-fixture-R-') as directory:
    control=Path(directory)
    (control/'approved-runtime-source.zip').write_bytes(f.git(root,'archive','--format=zip',spec['source'],'apps/platform','migrations'))
    files=f.verify_archive(root,control,spec)
    for name,content in files.items():
        path=control/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(content)
    env=os.environ.copy()
    for key in ['POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT']:
        env.pop(key,None)
    env.update(GOWORK='off',GOFLAGS='',CGO_ENABLED='0')
    base=control/'apps/platform'
    subprocess.run(['go','test','./internal/lifecycle/pilot','-run','TestRemaining(FixtureSchemaSingleContract|Retained|DeletePolicy)','-count=1'],cwd=base,env=env,check=True)
    subprocess.run(['go','test','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-remaining','./cmd/lifecycle-fixture-remaining-iam','./cmd/lifecycle-fixture-readonly-check','-count=1'],cwd=base,env=env,check=True)
print('PASS_SEPARATE_R_ARCHIVE_BUILD_AND_OFFLINE_CONTRACTS; no source/provider I/O')
