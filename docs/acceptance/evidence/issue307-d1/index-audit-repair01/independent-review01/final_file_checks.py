import ast,ctypes,hashlib,importlib.util,json,subprocess,sys
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[6];OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes
spec=importlib.util.spec_from_file_location('final_fileonly_entry',ROOT/'examples/lifecycle-d1/verify-six-native.py')
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
sha=lambda raw:hashlib.sha256(raw).hexdigest()
def save(name,obj):
    raw=(json.dumps(obj,indent=2,ensure_ascii=True)+'\n').encode()
    with (OUT/name).open('xb') as f:f.write(raw)
    return {'file':name,'sha256':sha(raw),'bytes':len(raw)}
def no_process(*a,**kw):raise AssertionError('review file-only check attempted real process')
folder=ROOT/'docs/acceptance/evidence/issue307-d1/index-audit-repair01'
plan=folder/'historical-evidence-audit-plan.json'
candidate=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate16/candidate.json'
with patch.object(subprocess,'Popen',no_process):
    audited=evidence_indexes.audit_plan(ROOT,plan,sha(plan.read_bytes()),candidate)
    saved=json.loads((folder/'actual-saved-evidence-audit01/evidence-audit.json').read_bytes())
    if audited!=saved:raise AssertionError('independent actual file audit differs from original saved receipt')
    if [x['members_verified'] for x in audited['indexes']]!=[53,19,12]:raise AssertionError('wrong actual member counts')
    actual_record=save('actual-saved-file-audit02.json',dict(audited,independently_recomputed=True,comparison_to_saved_original='EXACT_JSON_EQUAL',real_external_process_attempts=0))
    control=OUT/'terminal-parent-junction-counterexample01/control'
    with patch.object(v.r,'CONTROL',control):
        error=None
        try:v.require_unfinished_execution()
        except ValueError as exc:error=str(exc)
        if error is None:raise AssertionError('B1 still open')
    closure=save('terminal-parent-junction-recheck02.json',{'status':'CLOSED','parent_is_actual_junction':(control/'authorized-execution01').is_junction(),'error':error,'gate_entered':False,'source_provider_io':0,'external_process_attempts':0,'original_fixture_and_record_preserved':True})
    # Use an existing no-side-effect fixture for Windows short/long-name checks.
    f=OUT/'terminal-parent-junction-counterexample01/original-authorization'
    shortbuf=ctypes.create_unicode_buffer(32768)
    count=ctypes.windll.kernel32.GetShortPathNameW(str(f),shortbuf,len(shortbuf))
    if not count:raise OSError('GetShortPathNameW did not return a path')
    long=f.resolve(strict=True);short=Path(shortbuf.value)
    canonical=evidence_indexes.unlinked_absolute(short)
    if canonical!=long:raise AssertionError('short/long path canonicalization differs')
    short_check=save('actual-windows-path-normalization02.json',{'status':'PASS','original':str(short),'canonical':str(long),'distinct_spellings':str(short)!=str(long),'result':str(canonical),'root_links_checked_before_resolve':True,'source_provider_io':0,'real_external_process_attempts':0})
# Git file reads only; no Docker or provider command permitted.
def git(*args):return subprocess.check_output(['git','-c','core.longpaths=true',*args],cwd=ROOT)
head=git('rev-parse','HEAD').decode().strip()
expected='c1dd1b61495fd3150052744d6ba91e096a428b99'
if head!=expected:raise AssertionError('source HEAD changed during independent review')
paths=['.gitattributes','.github/workflows/ci.yml','.github/workflows/demo-lifecycle.yml','docs/acceptance/issue307-d1-index-audit-repair.md','examples/lifecycle-d1/evidence_indexes.py','examples/lifecycle-d1/verify-six-native.py','examples/lifecycle-d1/test_evidence_indexes.py','examples/lifecycle-d1/test_six_native_entry.py']
files={}
for name in paths:
    raw=(ROOT/name).read_bytes();blob=git('show',head+':'+name)
    normalized=raw.replace(b'\r\n',b'\n')
    if normalized!=blob.replace(b'\r\n',b'\n'):raise AssertionError('working/Git normalized bytes differ: '+name)
    files[name]={'sha256':sha(raw),'git_sha256':sha(blob),'bytes':len(raw),'git_bytes':len(blob),'raw_equals_git':raw==blob,'normalized_lf_equals_git':True}
base='74aa674db18dd68966d070e9f04173bb37ad1a7c'
def collect_ast(raw):
    tree=ast.parse(raw)
    return ast.dump(next(n for n in tree.body if isinstance(n,(ast.FunctionDef,ast.AsyncFunctionDef)) and n.name=='collect'),include_attributes=False)
collect_unchanged=collect_ast(git('show',base+':examples/lifecycle-d1/verify-six-native.py'))==collect_ast((ROOT/'examples/lifecycle-d1/verify-six-native.py').read_bytes())
if not collect_unchanged:raise AssertionError('original collector changed outside finite slice')
print(json.dumps({'source_readback':save('final-source-readback02.json',{'source_head':head,'base':base,'source_files':files,'collect_ast_unchanged_from_base':True}),'actual_saved_file_audit':actual_record,'B1_recheck':closure,'windows_normalization':short_check},indent=2))
