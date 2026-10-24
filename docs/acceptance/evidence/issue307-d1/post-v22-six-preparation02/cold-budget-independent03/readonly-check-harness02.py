from pathlib import Path
import subprocess,sys,runpy,json,hashlib
sys.dont_write_bytecode=True
root=Path.cwd();out=root/'docs/acceptance/evidence/issue307-d1/post-v22-six-preparation02/cold-budget-independent03'
original=subprocess.Popen;calls=[]
class ReadOnlyPopen(original):
 def __init__(self,args,*a,**kw):
  v=list(map(str,args));allow=False
  if kw.get('shell'):raise RuntimeError('shell forbidden')
  if v and Path(v[0]).name.lower() in ('git','git.exe'):allow=len(v)>1 and v[1] in ('cat-file','diff','ls-files','rev-parse','show')
  if v and Path(v[0]).name.lower() in ('docker','docker.exe'):
   allow=(len(v)>1 and v[1] in ('inspect','ps')) or (len(v)>2 and v[1:3] in (['network','inspect'],['volume','inspect']))
   if len(v)==4 and v[1]=='cp':allow=v[3]=='-' and v[2].split(':',1)[1] in ('/run/budget','/run/budget/.') and len(v[2].split(':')[0])==64
  if not allow:raise RuntimeError('readonly allowlist denied '+str(v[:3]))
  calls.append(v);super().__init__(args,*a,**kw)
subprocess.Popen=ReadOnlyPopen
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
sys.argv=[str(root/'examples/lifecycle-d1/six_candidate_runtime.py'),'check']
try:runpy.run_path(sys.argv[0],run_name='__main__')
finally:
 raw=(json.dumps({'calls':calls,'forbidden_mutations_allowed':False},indent=2)+'\n').encode()
 with (out/'readonly-subprocess-trace02.json').open('xb') as f:f.write(raw)
