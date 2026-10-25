"""Verify frozen reference closure and raw/LF/ZIP identities without modifying them."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import subprocess
import sys
import zipfile

ROOT = Path.cwd()
OUT = Path(__file__).resolve().parent
BASE = ROOT/'docs/acceptance/evidence/issue307-d1'
PRIVATE = Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate18')
PACKET = BASE/'remaining-candidate18'

def need(ok, message):
    if not ok:
        raise ValueError(message)

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def load(path):
    return json.loads(path.read_bytes())

def git(*args, **kwargs):
    return subprocess.check_output(['git', '-c', 'core.longpaths=true', *args], cwd=ROOT, **kwargs)

manifest_path = BASE/'candidate18-planning01/approval-reference-manifest01.json'
manifest = load(manifest_path); source = load(PACKET/'source-freeze.json'); candidate = load(PACKET/'candidate.json')
refs = manifest['approved_reference_artifacts']
need(len(refs) == manifest['count'] == 1017 and len({x['path'] for x in refs}) == 1017, 'Approval manifest count/membership differs')
refs = refs + [{'path': manifest_path.relative_to(ROOT).as_posix(), 'sha256': sha(manifest_path.read_bytes()), 'bytes': manifest_path.stat().st_size}]
head = git('rev-parse','HEAD').decode().strip()
need(head == 'a8930b31aa84fe0c003e2744253358e36c2f12f7', 'Published HEAD differs')
blobs = git('cat-file','--batch', input=''.join(head+':'+x['path']+'\n' for x in refs).encode())
offset = 0; dual = []; exact = 0
for item in refs:
    path = ROOT/item['path']; raw = path.read_bytes()
    need(sha(raw) == item['sha256'] and len(raw) == item['bytes'], 'Approved reference changed: '+item['path'])
    stop = blobs.index(b'\n', offset); header = blobs[offset:stop].split(); offset = stop+1
    need(len(header) == 3 and header[1] == b'blob', 'Git approved blob missing: '+item['path'])
    size = int(header[2]); blob = blobs[offset:offset+size]; offset += size+1
    need(blobs[offset-1:offset] == b'\n', 'Invalid cat-file boundary')
    if blob == raw:
        exact += 1
    else:
        frozen = source['files'].get(item['path'])
        need(frozen is not None and sha(raw) == frozen['raw_sha256'] and sha(blob) == frozen['lf_sha256'] and raw.replace(b'\r\n',b'\n') == blob, 'Approved raw/Git difference not covered by frozen source: '+item['path'])
        dual.append({'path': item['path'], 'raw_sha256': sha(raw), 'lf_git_sha256': sha(blob)})
need(offset == len(blobs) and exact == 1012 and len(dual) == 6, 'Approved Git closure differs')
archive = PRIVATE/'approved-runtime-source.zip'
need(sha(archive.read_bytes()) == source['archive_sha256'] and archive.stat().st_size == source['archive_bytes'], 'Actual source ZIP differs')
with zipfile.ZipFile(archive) as bundle:
    entries = bundle.infolist(); files = [x.filename for x in entries if not x.is_dir()]
    expected_zip_files = git('ls-tree','-r','--name-only',source['source'],'--','apps/platform','examples/lifecycle-d1','migrations').decode('utf-8','strict').splitlines()
    need(len(bundle.namelist()) == len(set(bundle.namelist())) and len(files) == len(expected_zip_files) and set(files) == set(expected_zip_files), 'Full source ZIP/Git tree membership differs')
    selected = {name for name in expected_zip_files if Path(name).suffix in {'.go','.py','.ps1','.sql','.json','.mod','.sum','.yml','.yaml','.Caddyfile'}}
    need(selected == set(source['files']) and len(selected) == 2129, 'Frozen generator selected source membership differs')
    for name, frozen in source['files'].items():
        raw = (ROOT/name).read_bytes(); data = bundle.read(name)
        need(sha(raw) == frozen['raw_sha256'] and sha(data) == frozen['lf_sha256'] and raw.replace(b'\r\n',b'\n') == data, 'Frozen ZIP/raw/LF source differs: '+name)
build = source['initial_build_files']
need(len(build) == 12 and all(sha((PRIVATE/name).read_bytes()) == digest for name,digest in build.items()), 'Original build member differs')
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import remaining_sequence
fields = ('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')
copies = [{k:cp[k] for k in fields} for _,_,cp in remaining_sequence.targets(candidate)]
target_digest = sha(json.dumps(copies,ensure_ascii=False,separators=(',',':')).encode())
need(target_digest == candidate['TargetDigest'], 'Recomputed exact six target digest differs')
statuses = []
for folder, expected in [('candidate18-source-review01','PASS_BOUNDED_C18_SOURCE_PREPARATION_REVIEW'),('candidate18-review01','PASS_BOUNDED_C18_FROZEN_NONDELETE_PREPARATION_REVIEW')]:
    path = BASE/folder/'independent-final-review01.json'; report = load(path)
    need(report['status'] == expected and report['remaining_blockers'] == [] and report['execution'] == candidate['Execution'] and report['source_head'] == source['source'], 'Independent review scope/blocker mismatch')
    statuses.append({'path':path.relative_to(ROOT).as_posix(),'sha256':sha(path.read_bytes()),'status':report['status'],'remaining_blockers':[]})
result = {'status':'PASS_APPROVAL_MANIFEST_GIT_SOURCE_ZIP_BUILD_AND_REVIEW_CLOSURE','observed_at':dt.datetime.now(dt.timezone.utc).isoformat(),
          'execution':candidate['Execution'],'head':head,'source':source['source'],'manifest_sha256':sha(manifest_path.read_bytes()),
          'approved_members':1017,'manifest_itself_verified':True,'total_Git_blobs_verified':len(refs),'exact_Git_raw_members':exact,
          'frozen_source_raw_LF_members':dual,'source_ZIP_selected_source_members':2129,'source_ZIP_total_files':len(files),'source_ZIP_directories':sum(x.is_dir() for x in entries),'source_ZIP_sha256':source['archive_sha256'],
          'source_ZIP_bytes':source['archive_bytes'],'original_build_members':12,'target_digest':target_digest,'independent_reports':statuses,
          'execution_authority':False,'source_start':0,'provider_io':0,'native_delete':0}
with (OUT/'manifest-build-review-closure01.json').open('xb') as stream:
    stream.write((json.dumps(result,ensure_ascii=False,indent=2)+'\n').encode())
print(json.dumps({k:v for k,v in result.items() if k not in ('frozen_source_raw_LF_members','independent_reports')}))
