"""Fresh physical old-budget reads even after their stopped wire is retired."""
import hashlib
import io
import subprocess
import tarfile
import retired_resources as r

def contents(raw,*,docker_copy):
    r.need(type(raw) is bytes,'budget archive must be bytes')
    rows={}
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:') as archive:
        for member in archive:
            parts=member.name.split('/')
            if docker_copy:
                r.need(parts[0]=='budget','original budget tar root differs');parts=parts[1:]
            elif parts[0]=='.':parts=parts[1:]
            r.need(not any(p in ('..','.') for p in parts),'unsafe budget archive path')
            key='/'.join(parts).rstrip('/')
            r.need(not key.startswith('/') and key not in rows and member.type in (tarfile.REGTYPE,tarfile.DIRTYPE),'duplicate or non-file budget member')
            value=dict(type=member.type.decode('ascii'),uid=member.uid,gid=member.gid,mode=member.mode,size=member.size)
            if member.isfile():value['sha256']=hashlib.sha256(archive.extractfile(member).read()).hexdigest()
            rows[key]=value
    r.need('' in rows and rows['']['type']=='5','budget root directory missing')
    return rows

def verify(plan,root):
    facts=[]
    for entry in plan['budgets']:
        name=entry['volume'];metadata=plan['volumes'][name]['metadata']
        r.need(metadata['Driver']=='local' and metadata['Scope']=='local' and not metadata['Options'],'budget is not a plain local volume')
        expected=contents(r.archive_file(root,entry['archive']),docker_copy=True)
        source='/mnt/docker-desktop-disk/data/docker/volumes/'+name+'/_data'
        start=r.now()
        result=subprocess.run(['wsl','-d','docker-desktop','--','tar','-cpf','-','-C',source,'.'],capture_output=True,timeout=60)
        r.need(result.returncode==0 and type(result.stdout) is bytes and type(result.stderr) is bytes,'fresh physical budget read failed')
        current=contents(result.stdout,docker_copy=False)
        r.need(current==expected,'retained physical budget bytes/ownership changed')
        facts.append(dict(volume=name,status='PASS_CURRENT_PHYSICAL_BUDGET_BYTES',started_at=start,observed_at=r.now(),tree_sha256=r.sha(r.encoded(current)),files=sum(row['type']=='0' for row in current.values())))
    return dict(status='PASS_ALL_RETIRED_AND_LIVE_BUDGET_BYTES',budgets=len(facts),observations=facts,source_started=0,budgets_refunded=0,budgets_reset=0)
