import asyncio
import hashlib
import importlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[6]
OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
SUITES=['test_remaining_six_window','test_six_candidate_runtime','test_freeze_six_candidate','test_verify_six_native','test_window_six_support','test_remaining_sequence','test_evidence_indexes','test_six_native_entry']
mode=sys.argv[1]
external=[]
def forbidden(*a,**kw):
    external.append(repr(a))
    raise AssertionError('INDEPENDENT REVIEW: real external process forbidden')
def save(name,raw):
    with (OUT/name).open('xb') as f:f.write(raw)
summary=[]
with patch.object(subprocess,'Popen',forbidden):
    for name in SUITES:
        mod=importlib.import_module(name)
        output=io.StringIO()
        suite=unittest.defaultTestLoader.loadTestsFromModule(mod)
        with patch('sys.stdout',output),patch('sys.stderr',output):
            result=unittest.TextTestRunner(stream=output,verbosity=2).run(suite)
        raw=output.getvalue().encode('utf-8')
        filename=mode+'-'+name+'.txt'
        save(filename,raw)
        item={'suite':name,'run':result.testsRun,'pass':result.testsRun-len(result.skipped)-len(result.errors)-len(result.failures),'skipped':[{'test':str(t),'reason':r} for t,r in result.skipped],'errors':len(result.errors),'failures':len(result.failures),'successful':result.wasSuccessful(),'log':filename,'sha256':hashlib.sha256(raw).hexdigest()}
        summary.append(item)
        print(json.dumps(item,ensure_ascii=True))
record={'mode':mode,'optimization':sys.flags.optimize,'tests':summary,'total':sum(x['run'] for x in summary),'pass':sum(x['pass'] for x in summary),'skip':sum(len(x['skipped']) for x in summary),'real_external_process_attempts':external,'source_provider_io':0,'native_delete':0,'claim':'UNIT_AND_RECORDED_DOUBLES_NOT_NATIVE'}
save(mode+'-summary.json',(json.dumps(record,indent=2,ensure_ascii=True)+'\n').encode())
if not all(x['successful'] for x in summary) or external:sys.exit(1)
