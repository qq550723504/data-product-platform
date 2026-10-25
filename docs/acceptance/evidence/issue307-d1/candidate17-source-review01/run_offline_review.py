import asyncio,hashlib,importlib,io,json,subprocess,sys,unittest
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
mode=sys.argv[1]
modules=['test_remaining_six_window','test_six_candidate_runtime','test_freeze_six_candidate','test_verify_six_native','test_window_six_support','test_remaining_sequence','test_evidence_indexes','test_six_native_entry','test_retained_v26']
external=[]
def reject(*args,**kw):external.append(repr(args));raise AssertionError('REAL EXTERNAL PROCESS FORBIDDEN IN INDEPENDENT OFFLINE REVIEW')
def save(name,raw):
    with (OUT/name).open('xb') as f:f.write(raw)
results=[]
with patch.object(subprocess,'Popen',reject):
    for name in modules:
        module=importlib.import_module(name);stream=io.StringIO()
        with patch('sys.stdout',stream),patch('sys.stderr',stream):result=unittest.TextTestRunner(stream=stream,verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(module))
        raw=stream.getvalue().encode();filename=mode+'-'+name+'.txt';save(filename,raw)
        item={'module':name,'methods':result.testsRun,'pass':result.testsRun-len(result.skipped)-len(result.errors)-len(result.failures),'errors':len(result.errors),'failures':len(result.failures),'skipped':[{'test':str(t),'reason':r} for t,r in result.skipped],'successful':result.wasSuccessful(),'log':filename,'sha256':hashlib.sha256(raw).hexdigest()}
        results.append(item);print(json.dumps(item))
record={'mode':mode,'optimization':sys.flags.optimize,'methods':sum(x['methods'] for x in results),'pass':sum(x['pass'] for x in results),'skip':sum(len(x['skipped']) for x in results),'suites':results,'unmocked_external_process_attempts':external,'native_Source_Docker_provider_DELETE':0}
save(mode+'-summary.json',(json.dumps(record,indent=2)+'\n').encode())
if external or not all(x['successful'] for x in results):sys.exit(1)
