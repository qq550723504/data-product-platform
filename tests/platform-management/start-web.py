"""Start an independently built management page, preserving all existing previews."""
import json,os,sys,subprocess,ssl,time,urllib.request
from pathlib import Path
from bootstrap import ROOT,RUN,spawn,env_clean
def main():
 if os.environ.get('MANAGEMENT_FIXTURE_ACK')!='ISOLATED_READONLY_MANAGEMENT':raise RuntimeError('explicit fixture required')
 if (RUN/'web-process.json').exists():raise RuntimeError('existing web ledger retained; refusing duplicate')
 import socket
 with socket.socket() as s:
  if s.connect_ex(('127.0.0.1',4300))==0:raise RuntimeError('management port occupied')
 import re
 dist=os.environ.get('MANAGEMENT_WEB_DIST','.next-acceptance-identity-management-20261025');web=ROOT/'apps/web'
 if not re.fullmatch(r'\.next-acceptance-identity-management-[a-z0-9-]+',dist):raise RuntimeError('independent build name required')
 if '--build' in sys.argv:
  config=web/'.tsconfig.acceptance-identity-management-fixture.json';original=(web/'next-env.d.ts').read_bytes()
  if config.exists():raise RuntimeError('temporary build config retained')
  config.write_bytes((web/'tsconfig.json').read_bytes())
  try:
   with open(RUN/'build.log','w',encoding='utf-8') as log:r=subprocess.run(['node','node_modules/next/dist/bin/next','build'],cwd=web,env={**env_clean(),'NEXT_DIST_DIR':dist,'NEXT_TSCONFIG_PATH':config.name,'NEXT_TELEMETRY_DISABLED':'1'},stdout=log,stderr=log)
   if r.returncode:raise RuntimeError('isolated management build failed; log retained')
  finally:(web/'next-env.d.ts').write_bytes(original);config.unlink()
 if not (web/dist/'BUILD_ID').exists():raise RuntimeError('completed independent build required')
 settings=json.loads((RUN/'web-env.json').read_text(encoding='utf-8'));settings['NEXT_DIST_DIR']=dist
 entry=spawn('web',['node',str(ROOT/'tests/platform-management/https-web.mjs')],settings);(RUN/'web-process.json').write_text(json.dumps(entry),encoding='utf-8')
 context=ssl.create_default_context(cafile=settings['MANAGEMENT_WEB_CERT'])
 for _ in range(120):
  try:
   with urllib.request.urlopen('https://localhost:4300/settings',context=context,timeout=2) as response:
    if response.status==200:break
  except Exception:time.sleep(.25)
 else:raise RuntimeError('owned management web unavailable; ledger retained')
 print('Independent modern management page: https://localhost:4300/settings; real sources UNCONFIGURED, business writes absent.')
if __name__=='__main__':main()
