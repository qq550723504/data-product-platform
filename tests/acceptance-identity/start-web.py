"""Start a previously built 1A web profile; never stops or resets anything."""
import json,os,subprocess,shutil,socket,time,ssl,urllib.request,re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
RUN=ROOT/'.data/acceptance-identity-1a'
def main():
 if os.environ.get('ACCEPTANCE_IDENTITY_ACK')!='ISOLATED_READONLY_1A':raise RuntimeError('explicit isolated 1A start required')
 if (RUN/'web-processes.json').exists():raise RuntimeError('existing identity process ledger; inspect/reuse it instead of duplicating servers')
 for port in [4290,4291]:
  with socket.socket() as probe:
   if probe.connect_ex(('127.0.0.1',port))==0:raise RuntimeError('identity port already owned; no process stopped')
 web=ROOT/'apps/web';name=os.environ.get('ACCEPTANCE_IDENTITY_WEB_DIST','.next-acceptance-identity-1a')
 if not re.fullmatch(r'\.next-acceptance-identity-(1a|1b(?:-[a-z0-9-]{1,40})?)',name):raise RuntimeError('explicit independent build required')
 dist=web/name;server=dist/'standalone/server.js'
 if dist.resolve().parent!=web.resolve():raise RuntimeError('build escaped independent workspace')
 if not server.is_file():raise RuntimeError('isolated standalone build required')
 # Copies stay under this new build; the retained .next build is untouched.
 shutil.copytree(dist/'static',server.parent/dist.name/'static',dirs_exist_ok=True)
 if (web/'public').exists():shutil.copytree(web/'public',server.parent/'public',dirs_exist_ok=True)
 env={**os.environ,**json.loads((RUN/'web-env.json').read_text(encoding='utf-8')),'PORT':'4291','HOSTNAME':'127.0.0.1'}
 env['ACCEPTANCE_WEB_CERT']=str(RUN/'server.crt');env['ACCEPTANCE_WEB_KEY']=str(RUN/'server.key')
 entries=[]
 for name,args,cwd in [('https',['node',str(ROOT/'tests/acceptance-identity/https-web.mjs')],ROOT)]:
  with open(RUN/(name+'.log'),'w',encoding='utf-8') as output:
   child=subprocess.Popen(args,cwd=cwd,env=env,stdout=output,stderr=output,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
  entries.append({'name':name,'pid':child.pid,'path':args[1],'startedAt':time.time()})
 (RUN/'web-processes.json').write_text(json.dumps(entries,indent=2),encoding='utf-8')
 context=ssl.create_default_context(cafile=str(RUN/'ca.crt'));deadline=time.monotonic()+45
 while time.monotonic()<deadline:
  try:
   with urllib.request.urlopen('https://localhost:4290/auth/login',context=context,timeout=2) as response:
    if response.status==200:break
  except Exception:time.sleep(0.25)
 else:raise RuntimeError('owned web did not become ready; ledger/log retained, no automatic restart')
 print('Independent identity web started at https://localhost:4290/identity. Retained preview was not stopped or rewritten.')
if __name__=='__main__':main()
