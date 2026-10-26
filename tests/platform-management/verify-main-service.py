"""Final read-only preservation comparison; never changes retained resources."""
import json,os,re,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
import bootstrap as b
if not (b.MAIN_SERVICE or b.COMPAT) or os.environ.get('MANAGEMENT_FIXTURE_ACK')!='ISOLATED_READONLY_MANAGEMENT':raise RuntimeError('independent main service fixture required')
name=os.environ.get('MANAGEMENT_MAIN_SERVICE_PRESERVATION_OUTPUT','retained-final-comparison')
if not re.fullmatch(r'retained-[a-z0-9-]+-comparison',name):raise RuntimeError('invalid preservation evidence name')
target=b.RUN/(name+'.json')
if target.exists():raise RuntimeError('final preservation report retained')
before=json.loads((b.RUN/'retained-before.json').read_text(encoding='utf-8'))
b.verify_retained(before,target,observer=b.metadata)
print('PASS retained container metadata; only read-only observations performed')
