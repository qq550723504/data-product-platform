"""Finite Docker Desktop Linux local-volume bridge; inspection only.

No directory creation, chmod, control enable or source start occurs here.
Tar/WSL/standard stat own filesystem inspection. UNC paths are a separate
explicit view of one inspected volume, never arbitrary POSIX path aliases.
"""
import io
import os
from pathlib import Path, PureWindowsPath
import re
import subprocess
import tarfile
from evidence_indexes import need
from retired_budget_bytes import contents


def paths(metadata):
    name=metadata.get('Name','')
    need(type(name) is str and re.fullmatch('d1-retention-[a-z0-9-]{1,100}',name),'maintenance volume name')
    need(metadata.get('Driver')=='local' and metadata.get('Scope')=='local' and not metadata.get('Options') and
         metadata.get('Labels')=={'dpp.retention-maintenance':'preparation-only'} and
         metadata.get('Mountpoint')=='/var/lib/docker/volumes/'+name+'/_data','maintenance plain local volume metadata')
    linux='/mnt/docker-desktop-disk/data/docker/volumes/'+name+'/_data'
    windows='\\\\wsl.localhost\\docker-desktop'+linux.replace('/','\\')
    return linux,windows


def wsl(*args):
    result=subprocess.run(['wsl','-d','docker-desktop','--',*args],capture_output=True,timeout=30)
    need(result.returncode==0 and type(result.stdout) is bytes,'maintenance Linux inspection failed')
    return result.stdout


def validate_directory(metadata, host_path, *, files=None, call=wsl):
    linux,windows=paths(metadata)
    need(os.name=='nt' and PureWindowsPath(host_path)==PureWindowsPath(windows),'maintenance UNC volume identity')
    # Parent components and every original entry are checked before resolving.
    from evidence_indexes import unlinked_absolute
    root=unlinked_absolute(host_path)
    raw=call('tar','-cpf','-','-C',linux,'.')
    return validate_snapshot(root,linux,raw,files=files,call=call)


def validate_snapshot(root,linux,raw,*,files=None,call=wsl):
    from evidence_indexes import unlinked_absolute
    tree=contents(raw,docker_copy=False)
    need(tree['']['uid']==10001 and tree['']['gid']==10001 and tree['']['mode']==0o700,'maintenance directory custody')
    current={name for name in tree if name}
    need(files is None or current==set(files),'maintenance file whitelist')
    need({p.name for p in root.iterdir()}==current,'maintenance UNC/Linux membership differs')
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:') as archive:
        for member in archive:
            name=member.name.removeprefix('./')
            if name in ('','.'):continue
            need('/' not in name and member.isfile() and member.uid==10001 and member.gid==10001 and member.mode==0o600,'maintenance file custody')
            path=unlinked_absolute(root/name)
            need(path.is_file() and path.stat().st_nlink==1 and path.read_bytes()==archive.extractfile(member).read(),'maintenance UNC/Linux file differs')
            # tar alone cannot detect an extra hardlink outside this directory.
            observed=call('stat','-c','%u:%g:%a:%h:%F',linux+'/'+name).decode('utf-8','strict').strip()
            need(observed=='10001:10001:600:1:regular file','maintenance Linux file identity/link differs')
    return tree
