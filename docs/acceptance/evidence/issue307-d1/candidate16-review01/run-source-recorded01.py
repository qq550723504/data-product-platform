import pathlib,sys,json,hashlib,unittest,unittest.mock,asyncio,subprocess,importlib.util,io,contextlib
out=pathlib.Path(__file__).resolve().parent;w=out.parents[4];original=subprocess.Popen
class NoExternalPopen(original):
 def __init__(self,*args,**kw):raise AssertionError('Independent source regression rejected external process')
subprocess.Popen=NoExternalPopen;mode='optimized' if sys.flags.optimize else 'normal';records=[]
try:
 for name in ('test_remaining_six_window','test_six_candidate_runtime','test_freeze_six_candidate','test_verify_six_native','test_window_six_support','test_remaining_sequence'):
  f=w/'examples/lifecycle-d1'/(name+'.py');stream=io.StringIO();spec=importlib.util.spec_from_file_location('independent_'+name,f);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  with contextlib.redirect_stdout(stream),contextlib.redirect_stderr(stream):result=unittest.TextTestRunner(stream=stream,verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(m))
  raw=stream.getvalue().encode();log=out/(name+'-'+mode+'01.txt');log.open('xb').write(raw);records.append({'suite':name,'tests':result.testsRun,'failures':len(result.failures),'errors':len(result.errors),'skipped':len(result.skipped),'PASS':result.wasSuccessful(),'source_sha256':hashlib.sha256(f.read_bytes()).hexdigest(),'log_sha256':hashlib.sha256(raw).hexdigest()})
finally:subprocess.Popen=original
record={'mode':mode,'suites':records,'total_methods':sum(x['tests'] for x in records),'all_pass':all(x['PASS'] for x in records),'actual_external_processes':0,'actual_Docker_Source_HTTP_enable_sign_DELETE':0};(out/('python-'+mode+'01.json')).open('xb').write(json.dumps(record,indent=2).encode()+b'\n');print(json.dumps(record));sys.exit(0 if record['all_pass'] else 1)
