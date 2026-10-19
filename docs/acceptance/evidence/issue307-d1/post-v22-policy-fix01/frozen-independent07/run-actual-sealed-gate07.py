import sys,pathlib,json,datetime,subprocess,importlib.util,asyncio,unittest.mock
root=pathlib.Path(__file__).resolve().parents[6];sys.path.insert(0,str(root/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('independent_actual_sealed07',root/'examples/lifecycle-d1/verify-six-native.py');v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
trace=[];blocked=[];original=subprocess.Popen
class ReadOnlyPopen(original):
 def __init__(self,args,*a,**kw):
  argv=[str(x) for x in args];allowed=argv[0]=='git' and argv[1] in ('cat-file','diff','ls-files','rev-parse','show')
  if argv[0]=='docker':allowed=argv[1]=='inspect' or argv[1:3] in (['volume','inspect'],['network','inspect']) or argv[1]=='ps' or (argv[1]=='cp' and argv[-1]=='-' and (argv[-2].endswith(':/run/budget') or argv[-2].endswith(':/run/budget/.')))
  if not allowed:blocked.append(argv);raise AssertionError('Independent read-only boundary rejected process')
  trace.append(argv);super().__init__(args,*a,**kw)
subprocess.Popen=ReadOnlyPopen
gate=v.sealed_gate()
result={'status':'PASS_ACTUAL_FROZEN_SEALED_GATE','at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'gate':gate,'read_only_process_count':len(trace),'unapproved_processes':blocked}
pathlib.Path(__file__).with_name('actual-sealed-process-trace07.json').open('xb').write(json.dumps(trace,indent=2).encode())
print(json.dumps(result))
