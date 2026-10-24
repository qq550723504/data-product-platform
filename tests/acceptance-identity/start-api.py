"""Build/start, or safely reload only the ledger-owned isolated API process."""
import json,os,subprocess,time,uuid,socket
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];RUN=ROOT/'.data/acceptance-identity-1a'
def main():
 if os.environ.get('ACCEPTANCE_IDENTITY_ACK')!='ISOLATED_READONLY_1A':raise RuntimeError('explicit isolated start required')
 target=RUN/('api-'+uuid.uuid4().hex+('.exe' if os.name=='nt' else ''))
 build=subprocess.run(['go','build','-o',str(target),'./tests/acceptanceidentity'],cwd=ROOT/'apps/platform',capture_output=True,encoding='utf-8')
 if build.returncode:raise RuntimeError('isolated API build failed; previous process retained')
 ledger=RUN/'api-process.json'
 if ledger.exists():
  old=json.loads(ledger.read_text(encoding='utf-8'));oldpath=Path(old['path']).resolve()
  if oldpath.parent!=RUN or not oldpath.name.startswith('api'):raise RuntimeError('invalid process owner')
  if os.name!='nt':raise RuntimeError('inspect existing process before reload on this platform')
  # No shell-built paths or destructive cross-shell operations. Start-time and
  # exact executable verification prevents killing an unrelated reused PID.
  script="""$p=Get-CimInstance Win32_Process -Filter ('ProcessId='+$env:DPP_OWN_PID);if(!$p -or $p.ExecutablePath -cne $env:DPP_OWN_PATH -or [Math]::Abs(([DateTimeOffset]$p.CreationDate).ToUnixTimeSeconds()-[double]$env:DPP_OWN_TIME)-gt 15){throw 'Owned API identity mismatch'};Stop-Process -Id $p.ProcessId -ErrorAction Stop"""
  e={**os.environ,'DPP_OWN_PID':str(old['pid']),'DPP_OWN_PATH':str(oldpath),'DPP_OWN_TIME':str(old.get('startedAt',ledger.stat().st_mtime))}
  subprocess.run(['powershell','-NoProfile','-NonInteractive','-Command',script],env=e,check=True,capture_output=True)
 with socket.socket() as probe:
  if probe.connect_ex(('127.0.0.1',18498))==0:raise RuntimeError('isolated API port remains owned; no other process stopped')
 env={**os.environ,**json.loads((RUN/'api-env.json').read_text(encoding='utf-8'))}
 with open(RUN/'api.log','w',encoding='utf-8') as log:child=subprocess.Popen([str(target)],cwd=ROOT,env=env,stdout=log,stderr=log,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
 ledger.write_text(json.dumps({'pid':child.pid,'path':str(target),'port':18498,'startedAt':time.time()}),encoding='utf-8')
 print('Owned isolated API updated. Original database and Agent host untouched.')
if __name__=='__main__':main()
