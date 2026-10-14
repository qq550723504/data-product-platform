"""Finite fresh-run composition of existing C2/B3 harness, no Core patches."""
import json, os, subprocess, sys
from pathlib import Path

repo = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(repo / 'examples/enterprise-activity/postgres-ingestion'))
import b3_runtime, c2_runtime, c2_connector_config

out = repo / '.data/independent299-runtime'
om = repo / '.data/independent299-om'
original = subprocess.Popen
prepared = False

def spawn(args, *pos, **kw):
    global prepared
    if not prepared and isinstance(args, list) and str(args[0]).endswith('pgi-b3-independent-api.exe'):
        prepared = True
        ownership = json.loads((out / 'ownership.json').read_text())
        private = Path(ownership['private'])
        cfg = json.loads((private / 'test.json').read_text())
        with (private / 'rights-bootstrap.log').open('wb') as log:
            r = subprocess.run([str(repo / '.data/pgi-b3-independent-fixture.exe'), 'rights'],
                env=dict(os.environ, TEST_INGESTION_B3_CONFIG=str(private / 'test.json')),
                stdout=log, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
        if r.returncode:
            raise RuntimeError('synthetic rights Command failed, see private log')
        public = json.loads((private / 'public.json').read_text())
        tmp = out / 'connector-runtime.json'
        tmp.write_text(json.dumps(dict(private=str(private), source=ownership['identity']+'-source',
            run_id=ownership['run_id'], workspaceId=public['workspaceId'], actorId=public['actorId'])))
        c2_connector_config.prepare(tmp, om)
        schema = json.loads((private / 'environment.json').read_text())['api']['OPENMETADATA_SCHEMA_PROFILE']
        kw['env']['OPENMETADATA_SCHEMA_PROFILE'] = schema
    p = original(args, *pos, **kw)
    if isinstance(args, list) and ('pgi-b3-independent-' in str(args[0]) or str(args[0]).lower() == 'cmd.exe'):
        with (repo / '.data/independent299/processes.jsonl').open('a', encoding='utf-8') as f:
            f.write(json.dumps({'pid':p.pid, 'command':list(map(str,args))})+'\n')
    return p

subprocess.Popen = spawn
c2_runtime.run(out, om, repo / '.data/pgi-b2-storage-cache',
    source_subnet='10.249.200.0/24', storage_subnet='10.249.201.0/24',
    binary_prefix='pgi-b3-independent')
