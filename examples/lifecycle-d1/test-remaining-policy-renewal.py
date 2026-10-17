"""Real PostgreSQL scratch regression only; no native provider, no cleanup."""
import importlib.util
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time

spec=importlib.util.spec_from_file_location('r',Path(__file__).with_name('prepare-remaining-runner.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)

def main():
    closure=sys.argv[1:]==['--closure-regression-on-retained-scratch']
    replay=sys.argv[1:]==['--repeat-on-retained-scratch'] or closure
    r.require(not sys.argv[1:] or replay,'Unsupported SQL regression invocation')
    private=r.PRIVATE/'remaining-policy-regression01';out=r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-policy-regression01'
    if replay:
        old=r.load((r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-policy-regression02' if closure else out)/'result.json');r.require(old['exit_code']==(0 if closure else 1),'Retained SQL test status differs')
        pg=old['container'];state=r.inspect(pg);r.require(state['Config']['Labels'].get('lifecycle307.scope')=='policy-regression01' and state['State']['Pid']==0,'Wrong scratch owner')
        password=(private/'password').read_text();port=int(state['HostConfig']['PortBindings']['5432/tcp'][0]['HostPort'])
        generation='03' if closure else '02'
        private=r.PRIVATE/('remaining-policy-regression'+generation);out=r.ROOT/('docs/acceptance/evidence/issue307-d1/remaining-policy-regression'+generation)
    r.require(not private.exists() and not out.exists(),'One-shot SQL regression allocated');private.mkdir();out.mkdir()
    name='dpp-lifecycle307-policy-regression01'
    if not replay:
        password=secrets.token_hex(32);(private/'password').write_text(password)
        with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
        r.docker('volume','create','--label','lifecycle307.scope=policy-regression01',name+'-data')
        pg=r.docker('create','--pull','never','--name',name,'--label','lifecycle307.scope=policy-regression01','--publish',f'127.0.0.1:{port}:5432','--mount','type=volume,source='+name+'-data,target=/var/lib/postgresql/data','--mount',f'type=bind,source={private}/password,target=/run/password,readonly','-e','POSTGRES_PASSWORD_FILE=/run/password',r.PG).decode().strip()
    try:
        r.docker('start',pg)
        for _ in range(80):
            if b'PostgreSQL init process complete; ready for start up.' in r.docker('logs',pg):break
            time.sleep(.5)
        r.native.wait_pg(pg,'postgres')
        env=os.environ.copy();env['TEST_POSTGRES_DSN']=f'postgres://postgres:{password}@127.0.0.1:{port}/postgres?sslmode=disable'
        pattern='TestCopyPolicyRenewal(FenceBothOrdersAndFrozenHistory|CrossScopeAndPendingOutputFailClosed)' if closure else 'Test(CopyPolicyRenewal|ArchiveRestoreCurrentWitnessAndRestrictions)'
        result=subprocess.run(['go','test','-race','-count=1','-v','./internal/lifecycle/application','-run',pattern],cwd=r.ROOT/'apps/platform',env=env,capture_output=True)
        raw=result.stdout+result.stderr;(private/'result.private-log').write_bytes(raw)
        (out/'tests.txt').write_bytes(raw.replace(password.encode(),b'[REDACTED]'))
        r.write(out/'result.json',{'container':pg,'source':r.call(['git','rev-parse','HEAD']).decode().strip(),'exit_code':result.returncode,'native_provider_configured':False,'native_delete':0,'retained_volume':name+'-data'})
        r.require(result.returncode==0,'SQL regression failed; retained diagnostics')
    finally:
        if r.inspect(pg)['State']['Running']:r.docker('stop','--time','20',pg)
        r.write(out/'seal.json',{'container':pg,'pid':r.inspect(pg)['State']['Pid'],'retained':True,'native_delete':0,'cleanup':0})

if __name__=='__main__':main()
