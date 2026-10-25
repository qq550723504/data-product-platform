import pathlib,json,hashlib,sys,unittest,unittest.mock,asyncio,subprocess,importlib.util,io,contextlib
out=pathlib.Path(__file__).resolve().parent;fix=pathlib.Path(r'C:\Users\Henry\.codex\worktrees\d1-cli-fix\data-product-platform');path=fix/'examples/lifecycle-d1/test_window_six_support.py';original=subprocess.Popen
class NoExternalPopen(original):
 def __init__(self,*args,**kw):raise AssertionError('Isolated repair independent test forbids every external process')
subprocess.Popen=NoExternalPopen;stream=io.StringIO();mode='optimized' if sys.flags.optimize else 'normal'
try:
 spec=importlib.util.spec_from_file_location('isolated_host_cli_tests',path);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
 with contextlib.redirect_stdout(stream),contextlib.redirect_stderr(stream):result=unittest.TextTestRunner(stream=stream,verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(m))
finally:subprocess.Popen=original
raw=stream.getvalue().encode();lp=out/('isolated-repair-'+mode+'01.txt');lp.open('xb').write(raw);summary={'mode':mode,'tests':result.testsRun,'failures':len(result.failures),'errors':len(result.errors),'skipped':len(result.skipped),'PASS':result.wasSuccessful(),'host_sha256':hashlib.sha256((fix/'examples/lifecycle-d1/window_six_support.py').read_bytes()).hexdigest(),'test_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'log_sha256':hashlib.sha256(raw).hexdigest(),'actual_external_processes':0,'actual_Docker_Source_provider_enable_sign_DELETE':0,'scope':'Isolated developer repair source only; frozen v25 unchanged, not activated'};(out/('isolated-repair-'+mode+'01.json')).open('xb').write(json.dumps(summary,indent=2).encode()+b'\n');print(json.dumps(summary));sys.exit(0 if result.wasSuccessful() else 1)
