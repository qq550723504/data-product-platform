"""Two explicit immutable artifacts: retained fixture Go R and PR/host P.

Git archive and Go provide packaging/building. No schema-dependent fallback,
runtime checkout switch, migration, provider call or authorization exists here.
"""
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import zipfile

SPEC = 'examples/lifecycle-d1/fixture-runtime-artifact.json'
HOST_PATHS = ('apps/platform','examples/lifecycle-d1','migrations','.github/workflows/ci.yml')

def need(value,message):
    if not value: raise ValueError(message)

def digest(raw): return hashlib.sha256(raw).hexdigest()
def load(path): return json.loads(Path(path).read_bytes())
def git(root,*args):
    env=os.environ.copy();env['TZ']='UTC'
    return subprocess.check_output(['git','-c','core.longpaths=true','-c','core.autocrlf=false',*args],cwd=root,env=env)

def runtime_archive(root,source):
    # ZIP DOS timestamps are local time; compression output may depend on
    # zlib. Native Git UTC + store-only fixes both without a custom archiver.
    return git(root,'archive','--format=zip','-0',source,'apps/platform','migrations')

def regular(path):
    need(path.is_file() and not path.is_symlink() and not path.is_junction()
         and path.stat().st_nlink==1,'artifact must be one regular unlinked file')

def archive_members(raw):
    values={}
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        for entry in archive.infolist():
            name=PurePosixPath(entry.filename)
            need(not name.is_absolute() and '..' not in name.parts and '\\' not in entry.orig_filename,
                 'unsafe runtime archive path')
            need((entry.external_attr >> 16)&0o170000 != 0o120000,'runtime archive symlink')
            if entry.is_dir():continue
            need(entry.filename not in values and entry.filename.startswith(('apps/platform/','migrations/')),
                 'runtime archive duplicate or foreign member')
            values[entry.filename]=archive.read(entry)
    need(values,'empty runtime archive')
    return values

def specification(root, *, execution, profile, start, end):
    path=Path(root)/SPEC;regular(path);spec=load(path)
    need(spec['version']==2 and spec['archive_recipe']=='GIT_ZIP_STORE_UTC_LF_V1'
         and spec['purpose']=='RETAINED_D1_FIXTURE_ONLY'
         and spec['execution']==execution and spec['profile']==profile
         and spec['not_before']==start and spec['not_after']==end,
         'runtime/host execution or window contract differs')
    need(spec['schema']=={'max_version':69,'migration_members':66,'protected_identity':False},
         'runtime fixture schema contract differs')
    need(re.fullmatch('[0-9a-f]{40}',spec['source']) is not None
         and git(root,'cat-file','-t',spec['source']).strip()==b'commit'
         and git(root,'rev-parse',spec['source']+'^').decode().strip()==spec['base_source'],
         'runtime source must be the one reviewed direct fixture commit')
    changed=git(root,'diff','--name-only',spec['base_source'],spec['source']).decode().splitlines()
    need(sorted(changed)==sorted(spec['runtime_delta_files']), 'runtime source contains unreviewed product delta')
    # Go consumes exactly these shared immutable scope/policy files; host P may
    # contain newer product repositories, but may not substitute this contract.
    for name in spec['shared_contract_files']:
        raw=(Path(root)/name).read_bytes().replace(b'\r\n',b'\n')
        need(digest(raw)==spec['files'][name], 'runtime/host shared contract drift')
    return spec

def verify_archive(root, control, spec):
    path=Path(control)/'approved-runtime-source.zip';regular(path);raw=path.read_bytes()
    need(len(raw)==spec['archive_bytes'] and digest(raw)==spec['archive_sha256'],
         'exact runtime archive drift')
    files=archive_members(raw)
    need({n:digest(v) for n,v in files.items()}==spec['files'], 'runtime archive membership/content drift')
    return files

def host_snapshot(root):
    need(not git(root,'diff','--name-only','HEAD','--',*HOST_PATHS).strip()
         and not git(root,'ls-files','--others','--exclude-standard','--',*HOST_PATHS).strip(),
         'commit reviewed host P before build/allocation')
    files={}
    for name in git(root,'ls-files','--',*HOST_PATHS).decode().splitlines():
        path=Path(root)/name;regular(path);raw=path.read_bytes()
        files[name]={'raw_sha256':digest(raw),'lf_sha256':digest(raw.replace(b'\r\n',b'\n'))}
    return {'source':git(root,'rev-parse','HEAD').decode().strip(),'files':files}

def verify_host(root, host):
    need(git(root,'cat-file','-t',host['source']).strip()==b'commit'
         and not git(root,'diff','--name-only',host['source'],'--',*HOST_PATHS).strip()
         and not git(root,'ls-files','--others','--exclude-standard','--',*HOST_PATHS).strip(),
         'host P tracked/untracked source drift')
    need(set(git(root,'ls-files','--',*HOST_PATHS).decode().splitlines())==set(host['files']),
         'host source membership drift')
    for name,expected in host['files'].items():
        path=Path(root)/name;regular(path);raw=path.read_bytes()
        need({'raw_sha256':digest(raw),'lf_sha256':digest(raw.replace(b'\r\n',b'\n'))}==expected,
             'host P raw/LF source drift')

def build(root, destination, contract):
    root,destination=Path(root),Path(destination)
    spec=specification(root,**contract);host=host_snapshot(root)
    archive=runtime_archive(root,spec['source'])
    with (destination/'approved-runtime-source.zip').open('xb') as stream:stream.write(archive)
    files=verify_archive(root,destination,spec)
    build_root=destination/'build-runtime';build_root.mkdir()
    for name,raw in files.items():
        path=build_root/name;path.parent.mkdir(parents=True,exist_ok=True)
        with path.open('xb') as stream:stream.write(raw)
    env=os.environ.copy();env.update(GOWORK='off',GOFLAGS='',GOARCH='amd64',CGO_ENABLED='0')
    recipes=[('reader','lifecycle-fixture-reader','linux'),('operator','lifecycle-fixture-remaining','linux'),
             ('owner','lifecycle-fixture-remaining-iam','linux'),('checker','lifecycle-fixture-readonly-check','windows')]
    for name,package,goos in recipes:
        env['GOOS']=goos
        output='readonly-check.exe' if name=='checker' else name
        result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(destination/output),
                               './cmd/'+package],cwd=build_root/'apps/platform',env=env,capture_output=True)
        for channel in ('stdout','stderr'):
            with (destination/(name+'-build.'+channel)).open('xb') as stream:stream.write(getattr(result,channel))
        need(result.returncode==0,'exact isolated R archive build failed')
    return {'source':spec['source'],'host':host,'runtime_artifact_spec_sha256':digest((root/SPEC).read_bytes()),
            'build_environment':{'GOWORK':'off','CGO_ENABLED':'0','GOARCH':'amd64'},
            'build_cwd_relative':'build-runtime/apps/platform'}

def verify(root,control,a,contract):
    spec=specification(root,**contract)
    need(a['source']==spec['source'] and a['runtime_artifact_spec_sha256']==digest((Path(root)/SPEC).read_bytes())
         and a['build_environment']=={'GOWORK':'off','CGO_ENABLED':'0','GOARCH':'amd64'}
         and a['build_cwd_relative']=='build-runtime/apps/platform','build source or recipe cross-binding')
    verify_archive(root,control,spec);verify_host(root,a['host'])
    return spec

def verify_frozen(packet,candidate,a,spec):
    frozen=load(Path(packet)/'source-freeze.json')
    need(digest((Path(packet)/'source-freeze.json').read_bytes())==candidate['SourceFreezeSHA256']
         and frozen['version']==2 and candidate['Source']==frozen['source']==a['source']
         and frozen['host']==a['host']
         and frozen['source_ci']==a['source_ci']
         and frozen['runtime_artifact_spec_sha256']==a['runtime_artifact_spec_sha256']
         and frozen['actual_build_source']==frozen['initial_allocation_build_source']==a['source']
         and frozen['initial_build_files']==a['build_files'], 'frozen R/P source/build binding differs')
    need(frozen['runtime']==spec and spec['source']==a['source'] and frozen['archive_sha256']==spec['archive_sha256']
         and frozen['archive_bytes']==spec['archive_bytes'],'frozen runtime archive cross-binding')
    return frozen
