"""Explicit one-time synthetic binding preparation in the retained isolated DB."""
import importlib.util,json,os,subprocess,urllib.parse
from pathlib import Path
root=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('binding_prepare',Path(__file__).with_name('prepare-readonly.py'))
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
def main():
 meta=json.loads(subprocess.check_output(['docker','inspect',p.CONTAINER],text=True))[0]
 assert meta['State']['Running'] and meta['Config']['Labels'].get('codex.scope')=='agent-q2-test'
 assert p.DATABASE=='agent_q2_acceptance_20261024'
 original=p.fingerprint('agent_q2_test')
 password=next(v.split('=',1)[1] for v in meta['Config']['Env'] if v.startswith('POSTGRES_PASSWORD='))
 env={**os.environ,'ACCEPTANCE_BINDING_PREPARE_ACK':'ISOLATED_SYNTHETIC_ONLY','READONLY_PREPARE_DSN':'postgres://postgres:'+urllib.parse.quote(password,safe='')+'@127.0.0.1:25494/'+p.DATABASE+'?sslmode=disable'}
 run=subprocess.run([str(root/'.data/agent-q2-preview/acceptance-bindings-prepare.exe')],cwd=root,env=env,capture_output=True,encoding='utf-8')
 (root/'.data/acceptance-bindings-prepare.log').write_text(run.stdout+run.stderr,encoding='utf-8')
 assert p.fingerprint('agent_q2_test')==original,'original changed'
 if run.returncode:raise RuntimeError('isolated binding preparation failed; inspect protected local log')
 print('Migration 74 and seven synthetic binding scenarios prepared; original fingerprint unchanged.')
if __name__=='__main__':main()
