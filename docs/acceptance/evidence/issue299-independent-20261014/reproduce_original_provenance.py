"""Reproduce minimum provenance solely from original full records; require exact old hashes."""
import hashlib,json,tarfile
from pathlib import Path

ROOT=Path(r'C:\Users\Henry\AppData\Local\Codex\retained-trials\dpp-pgi-b3-4iasa9v0\backup-old-oci-20261014')
ITEMS=[
 ('hop','131612631b5f764bbd62c6d5880dd0c9c2a126bbdf8d626ed0f98f19cead7b0f','ff1bd72c202898fc21b77a1f9f4cc0368c7dceea3b03ab76c842c4d69cd39a1f','pkg:docker/dpp-pgi-hop-jdbc@42.7.14?platform=linux%2Famd64','347e28663475d1744dea1790cbf44ae3ca9033a8a34f5788c0f33dff50244c27'),
 ('minio','2f6c8df7b4f4983f2ea3975c94bcecbfc8edc1bc35cf9659101184f46e7bbdd5','931486dc0b6c85a37a9f9621442c3b664ff7743ac5e8cbfad89ac35360ad2119','pkg:docker/dpp-pgi-minio@2025-10-15?platform=linux%2Famd64','21967b56fbd034e9a16e99dfecdff8da35f4a7d664c00a22a2c4db94f7b172b8')]

def main():
 out=ROOT/'provenance-reproduced';out.mkdir(exist_ok=False)
 rows=[]
 with tarfile.open(ROOT/'original-build-records.dockerbuild') as tar:
  for role,full,expected,subject,manifest in ITEMS:
   raw=tar.extractfile('blobs/sha256/'+full).read()
   assert hashlib.sha256(raw).hexdigest()==full
   predicate=json.loads(raw)
   predicate['buildDefinition']['internalParameters'].pop('buildConfig')
   meta=predicate['runDetails']['metadata']
   meta['buildkit_metadata']={k:v for k,v in meta['buildkit_metadata'].items() if k=='vcs'}
   statement={'_type':'https://in-toto.io/Statement/v1','predicateType':'https://slsa.dev/provenance/v1',
              'subject':[{'name':subject,'digest':{'sha256':manifest}}],'predicate':predicate}
   raw=json.dumps(statement,ensure_ascii=False,separators=(',',':')).replace('&',r'\u0026').replace('<',r'\u003c').replace('>',r'\u003e').encode()
   actual=hashlib.sha256(raw).hexdigest();assert actual==expected
   (out/(role+'.json')).write_bytes(raw)
   rows.append({'role':role,'fullOriginalPredicate':full,'minimumDigest':actual,'bytes':len(raw),'matchesOriginalManifest':True})
 (out/'receipt.json').write_text(json.dumps({'status':'PASS','rows':rows},indent=2)+'\n',encoding='utf-8',newline='\n')
 print(json.dumps({'status':'PASS','checks':len(rows)}))

if __name__=='__main__':main()
