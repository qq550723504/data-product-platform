"""Preserve current closed test-copy bytes too, including rehearsal-only rows."""
import hashlib
from pathlib import Path
import subprocess
import sys
import tarfile
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
ARCHIVE=Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01/capture06')

def normalized(name):return name.removeprefix('./').rstrip('/')

def main():
    plan=r.read(ARCHIVE/'volume-plan.json');out=ARCHIVE/'current-pg-copy-archives';out.mkdir()
    facts=[]
    for name,entry in plan['remove'].items():
        if entry['reason']!='REPRODUCIBLE_OLD_PG_RESTORE':continue
        folder=out/r.sha(name.encode());folder.mkdir()
        source='/mnt/docker-desktop-disk/data/docker/volumes/'+name+'/_data'
        start=r.now()
        with (folder/'data.tar.gz').open('xb') as target:
            result=subprocess.run(['wsl','-d','docker-desktop','--','tar','-czpf','-','-C',source,'.'],stdout=target,stderr=subprocess.PIPE,timeout=180)
            target.flush()
            import os;os.fsync(target.fileno())
        r.write(folder/'stderr',result.stderr);r.need(result.returncode==0,'closed test-copy archive failed')
        before=r.read(entry['content']['path'])
        expected={normalized(f['name']):{k:f[k] for k in ('type','size','uid','gid','mode','linkname') if k in f}|({'sha256':f['sha256']} if 'sha256' in f else {}) for f in before['files']}
        actual={}
        with tarfile.open(folder/'data.tar.gz','r:gz') as archive:
            for member in archive:
                row=dict(type=member.type.decode('ascii'),size=member.size,uid=member.uid,gid=member.gid,mode=member.mode,linkname=member.linkname)
                if member.isfile():
                    digest=hashlib.sha256();stream=archive.extractfile(member)
                    while block:=stream.read(1024*1024):digest.update(block)
                    row['sha256']=digest.hexdigest()
                key=normalized(member.name);r.need(key not in actual,'duplicate test-copy archive member');actual[key]=row
        r.need(actual==expected,'physical test-copy bytes/permissions differ from offline observation')
        fact=dict(volume=name,started_at=start,observed_at=r.now(),status='PASS_EXACT_CURRENT_PG_COPY_BYTES_ARCHIVED',
            content_members=len(actual),archive_sha256=r.sha((folder/'data.tar.gz').read_bytes()),archive_bytes=(folder/'data.tar.gz').stat().st_size,original_volume_unchanged=True,source_start=0)
        r.save(folder/'result.json',fact);facts.append(fact)
        print(r.encoded(dict(pg_copies_archived=len(facts),total=36)).decode(),flush=True)
    r.need(len(facts)==sum(e['reason']=='REPRODUCIBLE_OLD_PG_RESTORE' for e in plan['remove'].values()),'physical test-copy archive scope differs')
    files={path.relative_to(ARCHIVE).as_posix():dict(sha256=r.sha(path.read_bytes()),bytes=path.stat().st_size) for path in sorted(out.rglob('*')) if path.is_file()}
    r.save(ARCHIVE/'pg-copy-archive-index.json',dict(status='EXACT_RETIRED_PG_COPY_ARCHIVES',format='files-map-v1',count=len(files),files=files))
    r.save(ARCHIVE/'pg-copy-archive-result.json',dict(status='PASS_EXACT_CURRENT_TEST_PG_COPIES_ARCHIVED',copies=len(facts),compressed_bytes=sum(f['archive_bytes'] for f in facts),source_start=0,provider_calls=0))
if __name__=='__main__':main()
