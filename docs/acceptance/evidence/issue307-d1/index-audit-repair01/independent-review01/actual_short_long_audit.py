import ctypes,hashlib,json,subprocess,sys
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[6];OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit
plan=ROOT/'docs/acceptance/evidence/issue307-d1/index-audit-repair01/historical-evidence-audit-plan.json'
candidate=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate16/candidate.json'
sha=hashlib.sha256(plan.read_bytes()).hexdigest()
def short(path):
    buf=ctypes.create_unicode_buffer(32768)
    if not ctypes.windll.kernel32.GetShortPathNameW(str(path),buf,len(buf)):raise OSError('Windows short path unavailable')
    return Path(buf.value)
sroot,splan=short(ROOT),short(plan)
if str(sroot)==str(ROOT) or str(splan)==str(plan):raise AssertionError('fixture did not exercise different Windows spellings')
combinations=[]
with patch.object(subprocess,'Popen',side_effect=AssertionError('external process forbidden')):
    for root,pp in [(sroot,plan),(ROOT,splan),(sroot,splan)]:
        actual=audit.audit_plan(root,pp,sha,candidate)
        if actual['members_verified']!=84:raise AssertionError('actual saved member audit differs')
        combinations.append({'root':str(root),'plan':str(pp),'members_verified':actual['members_verified'],'index_counts':[x['members_verified'] for x in actual['indexes']]})
record={'status':'PASS_ACTUAL_WINDOWS_SHORT_LONG_AUDIT_PLAN','distinct_spellings':True,'combinations':combinations,'source_provider_io':0,'native_delete':0,'real_external_process_attempts':0}
raw=(json.dumps(record,indent=2)+'\n').encode()
with (OUT/'actual-short-long-audit-plan02.json').open('xb') as f:f.write(raw)
print(json.dumps({'sha256':hashlib.sha256(raw).hexdigest(),'combinations':len(combinations),'members_each':84}))
