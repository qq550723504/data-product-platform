"""Independently verify Docker's native save before publishing sanitized receipts."""
import hashlib,json,tarfile
from datetime import datetime,timezone
from pathlib import Path

HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
PUBLIC=REPO/'docs/acceptance/evidence/issue299-independent-20261014'
PRIVATE=Path(r'C:\Users\Henry\AppData\Local\Codex\retained-trials\dpp-pgi-b3-4iasa9v0\backup-old-oci-20261014')

def sha(path):
 with path.open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()

def sanitize(rows):
 return {name:{'Id':v['Id'],'Image':v['Image'],
   'State':{k:v['State'][k] for k in ['Status','Running','Paused','ExitCode','StartedAt','FinishedAt']},
   'Mounts':[{k:m[k] for k in ['Type','Name','Destination','RW'] if k in m} for m in v['Mounts']]}
   for name,v in rows.items()}

def main():
 recovered=json.loads((PRIVATE/'verified-original-graphs/recovery-final.json').read_text())
 assert recovered['status']=='PASS / EXACT_ORIGINAL_OCI_IDS_RESTORED'
 archive=PRIVATE/'verified-original-graphs/original-images.native.tar'
 expected={v['digest']:v for row in recovered['images'] for v in row['blobs']}
 verified=[]
 with tarfile.open(archive) as tar:
  root=json.load(tar.extractfile('index.json'))
  assert {d['digest'] for d in root['manifests']}=={row['originalImageId'] for row in recovered['images']}
  for dg,d in expected.items():
   m=tar.getmember('blobs/sha256/'+dg.split(':',1)[1]);assert m.size==d['bytes']
   with tar.extractfile(m) as f:actual=hashlib.file_digest(f,'sha256').hexdigest()
   assert 'sha256:'+actual==dg
   verified.append({'digest':dg,'bytes':m.size})
 native={'path':str(archive),'sha256':sha(archive),'bytes':archive.stat().st_size,
         'originalRootIndexes':sorted(d['digest'] for d in root['manifests']),
         'verifiedUniqueBlobs':len(verified),'everyOriginalDescriptorPresentAndDigestVerified':True}
 source={}
 for name in ['recover_original_oci.py','finalize_original_oci.py','reproduce_original_provenance.py','seal_image_recovery.py']:
  src=HERE/name;dst=PUBLIC/name
  dst.write_text(src.read_text(encoding='utf-8'),encoding='utf-8',newline='\n')
  source[name]={'executedSourceSha256':sha(src),'publicUTF8LFSha256':sha(dst)}
 provenance=json.loads((PRIVATE/'provenance-reproduced/receipt.json').read_text())
 ci=json.loads((HERE/'draft-pr319-ci-d9cecc1.json').read_text(encoding='utf-8-sig'))
 assert ci['headSha']=='d9cecc187e2d51609a208dafcde232495cc0d6c2' and ci['event']=='pull_request'
 assert ci['conclusion']=='success' and len(ci['jobs'])==11 and all(j['conclusion']=='success' for j in ci['jobs'])
 (PUBLIC/'draft-pr319-ci-d9cecc1.json').write_text(json.dumps(ci,indent=2)+'\n',encoding='utf-8',newline='\n')
 report={k:v for k,v in recovered.items() if k not in ['oldContainerStateBefore','oldContainerStateAfter']}
 report['oldContainerStateBefore']=sanitize(recovered['oldContainerStateBefore'])
 report['oldContainerStateAfter']=sanitize(recovered['oldContainerStateAfter'])
 report['oldContainerStateAndMountsUnchanged']=recovered['oldContainerStateBefore']==recovered['oldContainerStateAfter']
 report['sharedAliasesUnchanged']=recovered['sharedAliasesBefore']==recovered['sharedAliasesAfter']
 report['nativeSaveIndependentVerification']=native
 report['minimumProvenanceReproduction']=provenance
 report['helpers']=source
 report['originalFailedReceiptPreserved']='old-image-retention-defect.json'
 report['originalLostCore']='FAIL / NOT_RESTORED'
 report['humanExplanation']='NOT_RUN / deferred'
 report['sealedAt']=datetime.now(timezone.utc).isoformat()
 assert report['oldContainerStateAndMountsUnchanged'] and report['sharedAliasesUnchanged']
 (PUBLIC/'old-image-recovery.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8',newline='\n')
 (PRIVATE/'native-save-verification.json').write_text(json.dumps({'status':'PASS','native':native,'blobs':verified},indent=2)+'\n',encoding='utf-8',newline='\n')
 print(json.dumps({'status':report['status'],'uniqueOriginalBlobs':len(verified),'oldRuntimeUnchanged':True,'nativeSaveSha256':native['sha256']}))

if __name__=='__main__':main()
