"""Read closed original owners through docker cp; never mount/start a service."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import threading
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import retired_resources as r
BASE=Path('C:/Users/Henry/.codex/fixture-private/resource-retirement01')

def main():
    frozen=r.read(BASE/'budget-forensics01/all-containers.json')
    observed=r.read(BASE/'budget-forensics01/candidate-volumes.json')
    out=BASE/'capture05/volume-observations';out.mkdir(exist_ok=False)
    journal=out/'journal';journal.mkdir();docker=r.Docker(journal)
    names=[]
    for volume in observed['volumes']:
        name=volume['Name'];users=observed['users'][name]
        if any(u['destination']=='/run/budget' for u in users):continue
        names.append(name)
        user=users[0];cid=user['cid'];destination=user['destination']
        actual=docker.inspect([cid])[cid]
        r.need(r.closed(actual) and r.descriptor(actual)==r.descriptor(frozen[cid]),'volume reader changed')
        r.need(any(m['Type']=='volume' and m['Name']==name and m['Destination']==destination for m in actual['Mounts']),'unbound content path')
        folder=out/r.sha(name.encode());folder.mkdir()
        start=r.now();files=[]
        with (folder/'stderr').open('xb') as error:
            process=subprocess.Popen(['docker','cp',cid+':'+destination+'/.','-'],stdout=subprocess.PIPE,stderr=error)
            timer=threading.Timer(180,process.kill);timer.start()
            try:
                with tarfile.open(fileobj=process.stdout,mode='r|*') as archive:
                    for member in archive:
                        entry=dict(name=member.name,type=member.type.decode('ascii'),size=member.size,uid=member.uid,gid=member.gid,mode=member.mode,linkname=member.linkname)
                        if member.isfile():
                            digest=hashlib.sha256();stream=archive.extractfile(member)
                            while block:=stream.read(1024*1024):digest.update(block)
                            entry['sha256']=digest.hexdigest()
                        files.append(entry)
                code=process.wait(timeout=15)
            finally:timer.cancel();process.stdout.close()
        r.need(code==0,'volume readonly copy failed')
        r.save(folder/'content.json',dict(volume=name,reader=cid,destination=destination,started_at=start,observed_at=r.now(),exit_code=code,files=files,regular_bytes=sum(f['size'] for f in files if f['type']=='0'),source_start=0,provider_calls=0))
        if len(names)%20==0:print(json.dumps(dict(content_observed=len(names),total=160)),flush=True)
    r.save(out/'result.json',dict(status='PASS_EXACT_OFFLINE_VOLUME_CONTENT_OBSERVATION',observed=len(names),names=names,source_start=0,provider_calls=0))
    print(json.dumps(dict(content_observed=len(names),status='PASS')),flush=True)
if __name__=='__main__':main()
