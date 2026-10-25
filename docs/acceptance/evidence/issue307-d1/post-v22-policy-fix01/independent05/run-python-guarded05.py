import sys,unittest,subprocess,json,pathlib,datetime
root=pathlib.Path(__file__).resolve().parents[7]
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
blocked=[]
def deny(*args,**kwargs):
 blocked.append(str(args[0] if args else None));raise AssertionError('Independent boundary: unmodeled subprocess refused')
subprocess.Popen=deny
modules=['test_six_candidate_runtime','test_freeze_six_candidate','test_verify_six_native','test_window_six_support','test_remaining_six_window','test_remaining_sequence']
suite=unittest.defaultTestLoader.loadTestsFromNames(modules)
result=unittest.TextTestRunner(verbosity=2).run(suite)
print(json.dumps({'testsRun':result.testsRun,'failures':len(result.failures),'errors':len(result.errors),'skips':len(result.skipped),'unmodeled_subprocess_attempts':blocked,'optimized':not __debug__}))
sys.exit(0 if result.wasSuccessful() and not blocked else 1)
