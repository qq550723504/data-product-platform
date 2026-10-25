import ast,hashlib,importlib,json,re,subprocess,sys,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'));import evidence_indexes as audit
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate17')
BASE=ROOT/'docs/acceptance/evidence/issue307-d1';PACKET=BASE/'remaining-candidate17';PREP=BASE/'remaining-candidate17-preparation01';READ=BASE/'remaining-candidate17-readonly01';PLAN=BASE/'candidate17-planning01'
SOURCE='8cca345525af828a8c9461e75f2dc5bf5f1fb428';ANCHOR='462fae3a6cddde5417cbdf2cae4be1a384a5f1a0';EXEC='6f40bca9-cfe2-4362-ad2b-4e38bbbd8453'
sha=lambda raw:hashlib.sha256(raw).hexdigest()
def load(p):return json.loads(p.read_bytes())
def meta(p):raw=p.read_bytes();return {'sha256':sha(raw),'bytes':len(raw)}
def save(name,obj):
    raw=(json.dumps(obj,indent=2,ensure_ascii=True)+'\n').encode()
    with (OUT/name).open('xb') as f:f.write(raw)
def git(*args,input=None):return subprocess.check_output(['git','-c','core.longpaths=true',*args],cwd=ROOT,input=input)
assert git('rev-parse','HEAD').decode().strip()==ANCHOR
expected={'candidate.json':'20d08f0a2d334e779b64f0c082515131c285d9fd005c6fbaa94294b147f38d42','bindings.json':'77ce0be4d5dccd95a276a5b4c3f060689bea870f4f3caef1a827ef81154efa24','confirmation-template.txt':'7319519cb8d18cd024ac8b80bfe7552b27d858d5407c9e59ac8521afe1ab11c3'}
for name,digest in expected.items():assert sha((PACKET/name).read_bytes())==digest and (PACKET/name).read_bytes()==git('show',ANCHOR+':'+(PACKET/name).relative_to(ROOT).as_posix())
c=load(PACKET/'candidate.json');b=load(PACKET/'bindings.json');status=load(PACKET/'status.json')
assert c['Execution']==EXEC and c['Profile']=='candidate17/v27' and c['Source']==SOURCE and c['NotBefore']=='2026-10-21T06:00:00Z' and c['NotAfter']=='2026-10-21T06:40:00Z'
assert status['human_confirmation']=='NOT_RECEIVED' and status['schedule']=='NOT_CREATED' and status['permission_enabled'] is False and status['native_delete']==0
plan=PACKET/'preparation-evidence-audit-plan.json';audit_result=audit.audit_plan(ROOT,plan,sha(plan.read_bytes()),PACKET/'candidate.json');assert audit_result['members_verified']==1240
host=PLAN/'candidate17-host-bindings01.json';h=load(host)
assert h['count']==len(h['files'])==34 and h['source']==SOURCE and h['packet_commit']==ANCHOR and h['execution']==EXEC
host_result=audit.audit_index(ROOT,{'id':'host34','index':host.relative_to(ROOT).as_posix(),'sha256':sha(host.read_bytes()),'format':audit.FILES_MAP,'member_root':'.','expected_count':34,'expected_status':'FROZEN_V27_HOST_AND_PREPARATION_BINDINGS'})
archive=PRIVATE/'approved-runtime-source.zip';frozen=load(PACKET/'source-freeze.json')
assert sha(archive.read_bytes())==frozen['archive_sha256']==h['source_zip_sha256'] and archive.stat().st_size==frozen['archive_bytes']==h['source_zip_bytes']
assert frozen['source']==frozen['actual_build_source']==frozen['initial_allocation_build_source']==SOURCE and len(frozen['files'])==2117
names=list(frozen['files']);rawgit=git('cat-file','--batch',input=('\n'.join(SOURCE+':'+x for x in names)+'\n').encode());at=0;raw_git_equal=0;zip_git_equal=0;zip_lf_equal=0
with zipfile.ZipFile(archive) as z:
    allzip=z.namelist();assert len(allzip)==len(set(allzip))
    for name in names:
        line_end=rawgit.index(b'\n',at);header=rawgit[at:line_end].split();assert len(header)==3 and header[1]==b'blob',name
        size=int(header[2]);blob=rawgit[line_end+1:line_end+1+size];at=line_end+1+size+1
        raw=(ROOT/name).read_bytes();record=frozen['files'][name]
        assert sha(raw)==record['raw_sha256'] and sha(raw.replace(b'\r\n',b'\n'))==record['lf_sha256'] and sha(blob.replace(b'\r\n',b'\n'))==record['lf_sha256'],name
        zipped=z.read(name);assert zipped.replace(b'\r\n',b'\n')==blob.replace(b'\r\n',b'\n'),name
        raw_git_equal+=raw==blob;zip_git_equal+=zipped==blob;zip_lf_equal+=1
assert at==len(rawgit)
builds={}
for name,digest in frozen['initial_build_files'].items():
    p=PRIVATE/name;assert p.is_file() and not p.is_symlink() and not p.is_junction() and p.stat().st_nlink==1 and sha(p.read_bytes())==digest
    builds[name]=meta(p)
assert len(builds)==12 and builds['reader']['sha256']==c['ReaderBinarySHA256'] and builds['operator']['sha256']==c['SenderBinarySHA256'] and builds['owner']['sha256']==c['OwnerBinarySHA256']
anchor=load(PRIVATE/'host-anchor.json');anchor_readback=load(PLAN/'host-anchor-readback01.json')
assert anchor==anchor_readback['anchor'] and sha((PRIVATE/'host-anchor.json').read_bytes())==anchor_readback['private_anchor_sha256']
assert anchor['packet_commit']==ANCHOR and anchor['execution']==EXEC
for field,name in [('candidate_sha256','candidate.json'),('bindings_sha256','bindings.json'),('template_sha256','confirmation-template.txt')]:assert anchor[field]==expected[name]
# Preserve the review input matrix, not the mutable future publication matrix.
docsha={}
for name in ['issue307-d1-candidate17-confirmation.md','issue307-d1-v27-window-runbook.md','issue307-d1-v27-preparation.md']:
    raw=(ROOT/'docs/acceptance'/name).read_bytes();assert b'\xef\xbf\xbd' not in raw
    docsha[name]={'sha256':sha(raw),'bytes':len(raw)}
    if name.endswith('preparation.md'):
        with (OUT/'review-input-matrix.md').open('xb') as stream:stream.write(raw)
text=(ROOT/'docs/acceptance/issue307-d1-candidate17-confirmation.md').read_bytes();machine=text.split(b'```text\n',1)[1].split(b'```',1)[0]
assert machine==(PACKET/'confirmation-template.txt').read_bytes() and len(machine.splitlines())==115 and machine.endswith(b'\n') and b'\r' not in machine
assert machine.count('\u5220\u9664\u76ee\u6807 '.encode())==6 and machine.count('\u4fdd\u7559\u7248\u672c '.encode())==11
links=re.findall(rb'\[[^\]]*\]\(([^)]+)\)',text)
for link in links:
    value=link.decode();assert value.startswith(('https://','http://')) or (ROOT/'docs/acceptance'/value).exists(),value
windows=load(PACKET/'profile-windows.json');assert len(windows['cases'])==len(windows['recoveries'])==6
for row in windows['recoveries']:assert row['ValidUntil']=='2026-10-21T08:40:00Z'
assert c['ProfileWindowsSHA256']==sha((PACKET/'profile-windows.json').read_bytes()) and c['SourceFreezeSHA256']==sha((PACKET/'source-freeze.json').read_bytes())
report={'status':'PASS_FROZEN_FILE_GIT_ARCHIVE_PLAN_AND_DOCUMENT_BINDINGS','source':SOURCE,'packet_anchor':ANCHOR,'candidate_bindings_template':expected,'preparation_audit':audit_result,'host_audit':host_result,'source':{'files':2117,'working_raw_equals_Git_count':raw_git_equal,'all_working_LF_equals_Git':True,'zip_total_members':len(allzip),'zip_raw_equals_Git_selected_count':zip_git_equal,'zip_LF_equals_Git_selected_count':zip_lf_equal,'archive':meta(archive),'archive_raw_and_normalized_facts_are_distinct':True},'build_members':builds,'host_anchor':{'sha256':sha((PRIVATE/'host-anchor.json').read_bytes()),'packet_commit':ANCHOR,'authority':'CONTAINMENT_ONLY_NOT_SEND_AUTHORIZATION'},'documents':docsha,'confirmation_machine_lines':115,'confirmation_machine_equals_packet_raw':True,'relative_links_exist':len(links),'review_matrix_is_separate_snapshot':True,'private_secret_values_output':False,'Docker_Source_provider_DELETE':0}
save('frozen-file-binding-audit01.json',report);print(json.dumps({'status':report['status'],'source_files':2117,'plan_references':1240,'host_members':34,'raw_Git_equal':raw_git_equal,'ZIP_raw_Git_equal':zip_git_equal,'documents':docsha},indent=2))
