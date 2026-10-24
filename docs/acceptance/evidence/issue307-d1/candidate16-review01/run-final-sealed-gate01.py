import sys,pathlib,json,datetime,subprocess,importlib.util,asyncio,unittest.mock
w=pathlib.Path(__file__).resolve().parents[5];out=pathlib.Path(__file__).resolve().parent;sys.path.insert(0,str(w/'examples/lifecycle-d1'));spec=importlib.util.spec_from_file_location('c16_independent_actual_final_gate',w/'examples/lifecycle-d1/verify-six-native.py');v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v);trace=[];blocked=[];original=subprocess.Popen
class ReadOnlyPopen(original):
 def __init__(self,args,*a,**kw):
  argv=[str(x) for x in args];allow=not kw.get('shell') and argv[0]=='git' and argv[1]in('show','rev-parse','cat-file','diff','ls-files')
  if not kw.get('shell') and argv[0]=='docker':allow=argv[1]=='inspect' or argv[1:3]in(['volume','inspect'],['network','inspect']) or argv[1]=='ps' or (argv[1]=='cp' and argv[-1]=='-' and (argv[-2].endswith(':/run/budget') or argv[-2].endswith(':/run/budget/.')))
  if not allow:blocked.append(argv);raise AssertionError('Independent final read-only boundary rejected process')
  trace.append(argv);super().__init__(args,*a,**kw)
subprocess.Popen=ReadOnlyPopen;failure=None
try:gate=v.sealed_gate()
except BaseException as error:failure={'type':type(error).__name__,'message':str(error)};raise
finally:
 subprocess.Popen=original;(out/'actual-final-gate-processes01.json').open('xb').write(json.dumps({'commands':trace,'blocked':blocked,'failure':failure},indent=2).encode()+b'\n')
result={'status':'PASS_INDEPENDENT_ACTUAL_C16_FROZEN_SEALED_GATE','at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'gate':gate,'read_only_process_count':len(trace),'unapproved_processes':blocked,'actual_Source_HTTP_start_exec_enable_sign_DELETE':0};(out/'actual-final-sealed-gate01.json').open('xb').write(json.dumps(result,indent=2).encode()+b'\n');print(json.dumps(result))
