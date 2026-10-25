import sys,pathlib,json,hashlib,datetime,subprocess,importlib.util,asyncio,unittest.mock
root=pathlib.Path(__file__).resolve().parents[6];sys.path.insert(0,str(root/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('independent_c15_actual_runtime06',root/'examples/lifecycle-d1/six_candidate_runtime.py');r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
trace=[];blocked=[];original=subprocess.Popen
class ReadOnlyPopen(original):
 def __init__(self,args,*a,**kw):
  argv=[str(x) for x in args];allowed=argv[0]=='git' and argv[1] in ('cat-file','diff','ls-files','rev-parse','show')
  if argv[0]=='docker':
   allowed=argv[1]=='inspect' or argv[1:3] in (['volume','inspect'],['network','inspect']) or argv[1]=='ps' or (argv[1]=='cp' and argv[-1]=='-' and (argv[-2].endswith(':/run/budget') or argv[-2].endswith(':/run/budget/.')))
  if not allowed:blocked.append(argv);raise AssertionError('Independent read-only boundary rejected process')
  trace.append(argv);super().__init__(args,*a,**kw)
subprocess.Popen=ReadOnlyPopen
a=r.state();budget=r.budget_completion(a);gate=r.preparation_gate(a)
result={'status':'PASS_ACTUAL_STATE_COLD_PROFILE_BUDGET_GATE','at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'source':a['source'],'execution':a['execution'],'allocation_sha256':r.sha(r.PUBLIC/'allocation.json'),'new_owners':len(a['owners']),'retained_owners':len(a['retained']['closed_cids']),'gate':gate,'budget_receipt_sha256':r.sha(r.PUBLIC/'budget-initialization.json'),'read_only_process_count':len(trace),'unapproved_processes':blocked}
pathlib.Path(__file__).with_name('actual-readonly-process-trace06.json').open('xb').write((json.dumps(trace,indent=2)+'\n').encode())
print(json.dumps(result))
