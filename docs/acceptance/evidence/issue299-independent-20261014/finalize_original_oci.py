"""Finish exact OCI recovery with Docker's native canonical tagging."""
import base64, io, json, subprocess, tarfile
from datetime import datetime, timezone
import recover_original_oci as first

OUT=first.OUT
RECEIPT=OUT/'recovery-final.json'

def seal(r):RECEIPT.write_text(json.dumps(r,indent=2)+'\n',encoding='utf-8',newline='\n')

def main():
    assert not RECEIPT.exists()
    initial=json.loads((OUT/'recovery.json').read_text())
    r={'startedAt':datetime.now(timezone.utc).isoformat(),'status':'IN_PROGRESS',
       'priorHelperObservation':{'result':'FAIL at short tag lookup after successful native load',
          'originalHopIdQueryable':first.identity('sha256:'+first.ITEMS[0][2]),
          'cause':'OCI short ref-name was loaded as a noncanonical name; Docker CLI normalized tag lookup did not resolve it'},
       'originalBuildArchive':initial['originalBuildArchive'],
       'sharedAliasesBefore':initial['sharedAliasesBefore'],'oldContainerStateBefore':initial['oldContainerStateBefore'],
       'images':[],'oldContainersStarted':0,'oldVolumesMounted':0,'cleanupCalls':0,'newBuilds':0}
    seal(r)
    with tarfile.open(first.ROOT/'original-build-records.dockerbuild') as history:
        for name,ref,original,provenance_file,container in first.ITEMS:
            tag='dpp-restored-oci-'+name+':original-'+original[:16]
            missing=subprocess.run(['docker','image','inspect',tag],capture_output=True)
            assert missing.returncode!=0,'new canonical recovery tag required'
            row={'role':name,'buildRef':ref,'originalImageId':'sha256:'+original,'tag':tag,'blobs':[]}
            r['images'].append(row);seal(r)
            blobs=OUT/'blobs'/'sha256'
            root_bytes=history.extractfile('blobs/sha256/'+original).read()
            root=blobs/original
            if not root.exists():root.write_bytes(root_bytes)
            queue=[{'mediaType':'application/vnd.oci.image.index.v1+json','digest':'sha256:'+original,'size':len(root_bytes)}]
            checked=set();provenance=(first.ROOT/provenance_file).read_bytes()
            while queue:
                desc=queue.pop(0);dg=desc['digest'];hexid=dg.split(':',1)[1]
                assert dg.startswith('sha256:') and len(hexid)==64
                if dg in checked:continue
                path=blobs/hexid;source='retained-buildkit-content'
                if not path.exists():
                    if 'data' in desc:path.write_bytes(base64.b64decode(desc['data']))
                    elif first.hashlib.sha256(provenance).hexdigest()==hexid:path.write_bytes(provenance)
                    else:
                        with path.open('xb') as f:
                            proc=subprocess.run(['docker','buildx','history','inspect','attachment',ref,dg],stdout=f,stderr=subprocess.PIPE,timeout=240)
                        (OUT/(name+'-'+hexid+'.stderr')).write_bytes(proc.stderr)
                        assert proc.returncode==0,proc.stderr.decode(errors='replace')
                if 'data' in desc:source='original-inline-descriptor'
                if first.hashlib.sha256(provenance).hexdigest()==hexid:source='exact-digest-minimum-from-original-full-provenance'
                if hexid==original:source='original-build-history-index'
                assert path.stat().st_size==desc['size'] and first.digest_file(path)==hexid
                checked.add(dg);row['blobs'].append({'digest':dg,'bytes':desc['size'],'mediaType':desc['mediaType'],'source':source})
                if 'image.index' in desc['mediaType']:queue.extend(json.loads(path.read_bytes())['manifests'])
                elif 'image.manifest' in desc['mediaType']:
                    manifest=json.loads(path.read_bytes());queue.extend([manifest['config'],*manifest['layers']])
                seal(r);print(json.dumps({'role':name,'verified':dg,'bytes':desc['size']}),flush=True)
            archive=OUT/(name+'-original-complete.oci.tar')
            wrapper={'schemaVersion':2,'mediaType':'application/vnd.oci.image.index.v1+json',
                     'manifests':[{'mediaType':'application/vnd.oci.image.index.v1+json','digest':'sha256:'+original,'size':len(root_bytes)}]}
            with tarfile.open(archive,'w') as tar:
                for member,data in [('oci-layout',b'{"imageLayoutVersion":"1.0.0"}'),('index.json',json.dumps(wrapper,separators=(',',':')).encode())]:
                    info=tarfile.TarInfo(member);info.size=len(data);tar.addfile(info,io.BytesIO(data))
                for dg in sorted(checked):tar.add(blobs/dg.split(':',1)[1],arcname='blobs/sha256/'+dg.split(':',1)[1],recursive=False)
            row['archive']={'path':str(archive),'bytes':archive.stat().st_size,'sha256':first.digest_file(archive)}
            row['allOriginalDescriptorsVerified']=True;seal(r)
            row['nativeLoadOutput']=first.run(['docker','image','load','--input',str(archive)],timeout=240).decode().strip()
            assert first.identity('sha256:'+original)=='sha256:'+original
            first.run(['docker','image','tag','sha256:'+original,tag])
            row['restoredImageId']=first.identity(tag)
            assert row['restoredImageId']=='sha256:'+original
            row['status']='PASS / EXACT_ORIGINAL_OCI_ID_RESTORED';seal(r)
    r['sharedAliasesAfter']={a:first.identity(a) for a in first.ALIASES}
    r['oldContainerStateAfter']=first.retained_state()
    assert r['sharedAliasesAfter']==r['sharedAliasesBefore']
    assert r['oldContainerStateAfter']==r['oldContainerStateBefore']
    r['completedAt']=datetime.now(timezone.utc).isoformat();r['status']='PASS / EXACT_ORIGINAL_OCI_IDS_RESTORED';seal(r)
    print(json.dumps({'status':r['status'],'images':[row['restoredImageId'] for row in r['images']]}),flush=True)

if __name__=='__main__':main()
