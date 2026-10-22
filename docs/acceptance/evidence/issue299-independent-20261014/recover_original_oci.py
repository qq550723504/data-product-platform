"""Recover two original OCI graphs from retained BuildKit content. No old runtime start."""
import base64, hashlib, io, json, subprocess, tarfile
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(r'C:\Users\Henry\AppData\Local\Codex\retained-trials\dpp-pgi-b3-4iasa9v0\backup-old-oci-20261014')
OUT = ROOT / 'verified-original-graphs'
ITEMS = [
    ('hop', 'jz6yudwfhfayrfa21eg88wwco', '401553bcfe449470f33e8efb5ce4414dec4e880eec357b37738d41d0c2b6df18', 'hop-minimum-provenance-exact.json', 'dpp-pgi-70ca779a1e21-hop'),
    ('minio', 'mi4g4leqqgsnz4ze4qwk9jp9o', '8c712a2fe14b096ebb65f17c976a3aff15857ca1ad63a0cef250fdc22771202b', 'minio-minimum-provenance-candidate.json', 'dpp-pgi-b3-storage-70ca779a1e21'),
]
ALIASES = ['dpp-pgi-hop-jdbc:42.7.14', 'dpp-pgi-minio:2025-10-15']

def run(args, timeout=120):
    r = subprocess.run(args, capture_output=True, timeout=timeout)
    assert r.returncode == 0, r.stderr.decode(errors='replace')
    return r.stdout

def digest_file(path):
    with path.open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()

def identity(ref):
    return json.loads(run(['docker', 'image', 'inspect', ref]))[0]['Id']

def retained_state():
    rows = json.loads(run(['docker', 'inspect', *[x[4] for x in ITEMS]]))
    return {r['Name']: {k:r[k] for k in ['Id','Image','State','Mounts']} for r in rows}

def seal(receipt):
    (OUT / 'recovery.json').write_text(json.dumps(receipt, indent=2)+'\n', encoding='utf-8', newline='\n')

def main():
    OUT.mkdir(exist_ok=False)
    blobs = OUT / 'blobs' / 'sha256'; blobs.mkdir(parents=True)
    before_aliases = {a:identity(a) for a in ALIASES}
    before_state = retained_state()
    receipt = {'startedAt':datetime.now(timezone.utc).isoformat(),'status':'IN_PROGRESS',
               'originalBuildArchive':{'path':str(ROOT/'original-build-records.dockerbuild'),'sha256':digest_file(ROOT/'original-build-records.dockerbuild')},
               'sharedAliasesBefore':before_aliases,'oldContainerStateBefore':before_state,'images':[],
               'oldContainersStarted':0,'oldVolumesMounted':0,'cleanupCalls':0,'newBuilds':0}
    seal(receipt)
    with tarfile.open(ROOT/'original-build-records.dockerbuild') as history:
        for name, ref, original, provenance_file, container in ITEMS:
            tag = 'dpp-retained-oci-'+name+':original-'+original[:16]
            exists = subprocess.run(['docker','image','inspect',tag],capture_output=True)
            assert exists.returncode != 0, 'recovery tag must be new'
            row = {'role':name,'buildRef':ref,'originalImageId':'sha256:'+original,'tag':tag,'blobs':[]}
            receipt['images'].append(row); seal(receipt)
            root_bytes = history.extractfile('blobs/sha256/'+original).read()
            assert hashlib.sha256(root_bytes).hexdigest() == original
            (blobs/original).write_bytes(root_bytes)
            queue = [{'mediaType':'application/vnd.oci.image.index.v1+json','digest':'sha256:'+original,'size':len(root_bytes)}]
            checked=set()
            provenance = (ROOT/provenance_file).read_bytes()
            while queue:
                desc = queue.pop(0); dg = desc['digest']; hexid = dg.split(':',1)[1]
                assert dg.startswith('sha256:') and len(hexid)==64
                if dg in checked: continue
                path=blobs/hexid; source='retained-buildkit-content'
                if not path.exists():
                    if 'data' in desc:
                        path.write_bytes(base64.b64decode(desc['data'])); source='original-inline-descriptor'
                    elif hashlib.sha256(provenance).hexdigest()==hexid:
                        path.write_bytes(provenance); source='exact-digest-minimum-from-original-full-provenance'
                    else:
                        with path.open('xb') as f:
                            r=subprocess.run(['docker','buildx','history','inspect','attachment',ref,dg],stdout=f,stderr=subprocess.PIPE,timeout=240)
                        (OUT/(name+'-'+hexid+'.stderr')).write_bytes(r.stderr)
                        assert r.returncode==0,r.stderr.decode(errors='replace')
                assert path.stat().st_size==desc['size'], 'original descriptor size mismatch'
                assert digest_file(path)==hexid,'original descriptor digest mismatch'
                checked.add(dg)
                row['blobs'].append({'digest':dg,'bytes':desc['size'],'mediaType':desc['mediaType'],'source':source})
                if 'image.index' in desc['mediaType']:
                    queue.extend(json.loads(path.read_bytes())['manifests'])
                elif 'image.manifest' in desc['mediaType']:
                    manifest=json.loads(path.read_bytes()); queue.extend([manifest['config'],*manifest['layers']])
                seal(receipt)
                print(json.dumps({'role':name,'verified':dg,'bytes':desc['size']}),flush=True)
            archive=OUT/(name+'-original.oci.tar')
            wrapper={'schemaVersion':2,'mediaType':'application/vnd.oci.image.index.v1+json',
                     'manifests':[{'mediaType':'application/vnd.oci.image.index.v1+json','digest':'sha256:'+original,'size':len(root_bytes),'annotations':{'org.opencontainers.image.ref.name':tag}}]}
            with tarfile.open(archive,'w') as tar:
                for member,data in [('oci-layout',b'{"imageLayoutVersion":"1.0.0"}'),('index.json',json.dumps(wrapper,separators=(',',':')).encode())]:
                    info=tarfile.TarInfo(member);info.size=len(data);tar.addfile(info,io.BytesIO(data))
                for dg in sorted(checked):tar.add(blobs/dg.split(':',1)[1],arcname='blobs/sha256/'+dg.split(':',1)[1],recursive=False)
            row['archive']={'path':str(archive),'bytes':archive.stat().st_size,'sha256':digest_file(archive)}
            row['allOriginalDescriptorsVerified']=True;seal(receipt)
            row['nativeLoadOutput']=run(['docker','image','load','--input',str(archive)],timeout=240).decode().strip()
            row['restoredImageId']=identity(tag)
            assert row['restoredImageId']=='sha256:'+original
            assert identity('sha256:'+original)=='sha256:'+original
            row['status']='PASS / EXACT_ORIGINAL_OCI_ID_RESTORED';seal(receipt)
    receipt['sharedAliasesAfter']={a:identity(a) for a in ALIASES}
    receipt['oldContainerStateAfter']=retained_state()
    assert receipt['sharedAliasesAfter']==before_aliases
    assert receipt['oldContainerStateAfter']==before_state
    receipt['completedAt']=datetime.now(timezone.utc).isoformat()
    receipt['status']='PASS / EXACT_ORIGINAL_OCI_IDS_RESTORED';seal(receipt)
    print(json.dumps({'status':receipt['status'],'images':[r['restoredImageId'] for r in receipt['images']]}),flush=True)

if __name__=='__main__':main()
