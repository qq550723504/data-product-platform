"""Author's non-native regression receipt; exact channels are append-only."""
import json
from pathlib import Path
import re
import subprocess

root=Path(__file__).resolve().parents[5]
output=Path(__file__).with_name('source-final04')
output.mkdir(exist_ok=False)
results=[]
for mode,flags in [('normal',[]),('optimized',['-O'])]:
    methods=skips=0
    for name in ['test_remaining_sequence.py','test_remaining_six_window.py','test_six_candidate_runtime.py','test_freeze_six_candidate.py','test_window_six_support.py','test_verify_six_native.py','test_evidence_indexes.py','test_six_native_entry.py','test_retained_v26.py']:
        result=subprocess.run(['python',*flags,'-B',str(root/'examples/lifecycle-d1'/name)],cwd=root,capture_output=True)
        for channel in ('stdout','stderr'):
            with (output/(mode+'-'+name+'.'+channel)).open('xb') as stream:stream.write(getattr(result,channel))
        text=result.stderr.decode('utf-8','strict');count=re.search(r'Ran (\d+) tests?',text);skip=re.search(r'skipped=(\d+)',text)
        methods+=int(count[1]) if count else 0;skips+=int(skip[1]) if skip else 0
        results.append({'mode':mode,'test':name,'exit':result.returncode})
    results.append({'mode':mode,'methods':methods,'skips':skips})
go=subprocess.run(['go','test','-json','./internal/lifecycle/pilot','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-readonly-check','./cmd/lifecycle-fixture-remaining','./cmd/lifecycle-fixture-remaining-iam'],cwd=root/'apps/platform',capture_output=True)
for channel in ('stdout','stderr'):
    with (output/('go.'+channel)).open('xb') as stream:stream.write(getattr(go,channel))
rows=[json.loads(line) for line in go.stdout.splitlines() if line.startswith(b'{')]
results.append({'mode':'go','exit':go.returncode,'tests':{kind:sum(x.get('Action')==kind and 'Test' in x for x in rows) for kind in ('pass','fail','skip')}})
with (output/'summary.json').open('x',encoding='utf-8') as stream:json.dump(results,stream,indent=2)
print(json.dumps(results))
raise SystemExit(1 if any(x.get('exit',0) for x in results) else 0)
