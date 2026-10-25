import sys,pathlib,io,contextlib,json,subprocess,asyncio,unittest,unittest.mock,importlib.util,hashlib
w=pathlib.Path(__file__).resolve().parents[5];out=pathlib.Path(__file__).resolve().parent
original=subprocess.Popen;trace=[]
class ReadOnlyPopen(original):
 def __init__(self,args,*a,**kw):
  argv=[str(x) for x in args]
  if kw.get('shell') or argv[0]!='git' or argv[1] not in ('show','rev-parse','cat-file','diff','ls-files'):raise AssertionError('Independent mock boundary rejected external process: '+repr(argv[:3]))
  trace.append(argv);super().__init__(args,*a,**kw)
subprocess.Popen=ReadOnlyPopen
label='optimized' if sys.flags.optimize else 'normal';summaries=[]
try:
 for name,path in [('six',w/'examples/lifecycle-d1/test_window_six_support.py'),('retained',w/'docs/acceptance/evidence/issue307-d1/remaining-window-review06/composition/test_window_support.py')]:
  stream=io.StringIO();spec=importlib.util.spec_from_file_location('review_'+name,path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
  with contextlib.redirect_stdout(stream),contextlib.redirect_stderr(stream):result=unittest.TextTestRunner(stream=stream,verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(m))
  raw=stream.getvalue().encode();(out/(name+'-'+label+'01.txt')).open('xb').write(raw)
  summaries.append({'suite':name,'tests':result.testsRun,'failures':len(result.failures),'errors':len(result.errors),'skipped':len(result.skipped),'pass':result.wasSuccessful(),'source_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'log_sha256':hashlib.sha256(raw).hexdigest()})
finally:subprocess.Popen=original
summary={'mode':label,'suites':summaries,'actual_subprocesses':trace,'actual_Docker_Source_provider_enable_sign_DELETE':0,'temporary_mock_signing_only':True};(out/(label+'-mock-suites01.json')).open('xb').write(json.dumps(summary,indent=2).encode()+b'\n');print(json.dumps(summary));sys.exit(0 if all(x['pass'] for x in summaries) else 1)
