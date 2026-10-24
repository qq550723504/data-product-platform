"""Actual R-archive build and fixed schema gate on isolated PG copies only."""
from pathlib import Path
import importlib.util
import json
import os
import subprocess
import sys
import time

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import fixture_artifacts as f
loader=importlib.util.spec_from_file_location('isolated_clone_primitives',Path(__file__).with_name('rehearse-schema-compatibility01.py'))
m=importlib.util.module_from_spec(loader);loader.loader.exec_module(m)
OUT=Path(__file__).parent/'artifact-rehearsal01'
CONTROL=m.PRIVATE/'candidate20-artifact-rehearsal01'
CLONE=m.PRIVATE/'candidate20-schema-compatibility01'

def main():
    m.need(not OUT.exists() and not CONTROL.exists(),'one-shot artifact rehearsal already exists')
    OUT.mkdir();CONTROL.mkdir();(CONTROL/'journal').mkdir();m.CONTROL=CONTROL
    contract=dict(execution=m.EXECUTION,profile='candidate20/v30',start='2026-10-22T10:00:00Z',end='2026-10-22T10:40:00Z')
    artifact=f.build(ROOT,CONTROL,contract)
    artifact['build_files']={n:m.sha(CONTROL/n) for n in ['reader','operator','owner','readonly-check.exe',*[n+'-build.'+c for n in ['reader','operator','owner','checker'] for c in ['stdout','stderr']]]}
    m.save(OUT/'actual-build.json',artifact);f.verify(ROOT,CONTROL,artifact,contract)
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0',GOWORK='off',GOFLAGS='')
    test=CLONE/'tools/fixture-schema.test'
    m.need(not test.exists(),'original clone tools cannot be overwritten')
    build=subprocess.run(['go','test','-c','-o',str(test),'./internal/lifecycle/pilot'],cwd=CONTROL/'build-runtime/apps/platform',env=env,capture_output=True)
    m.raw(OUT/'schema-test-build.stdout.txt',build.stdout);m.raw(OUT/'schema-test-build.stderr.txt',build.stderr)
    m.need(build.returncode==0,'R archive schema test build failed')
    core=m.load(CLONE/'allocation.json')['cid'];old=m.load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate19/candidate.json')
    originals=[g[k] for g in old['Groups'] for k in ('Core','Native')]
    m.need(all(m.closed(m.inspect(cid)) for cid in originals),'all actual sources must remain stopped')
    x=m.inspect(core);m.need(m.closed(x) and x['HostConfig']['NetworkMode']=='none' and x['Config']['Labels'].get('lifecycle307.schema-rehearsal')==m.EXECUTION,'isolated clone identity differs')
    result=dict(runtime_source=artifact['source'],host_source=artifact['host']['source'],schema_test_sha256=m.sha(test),groups=[],source_started=False,source_migration=False,native_delete=0)
    failure=None
    try:
        m.docker('start',core)
        for _ in range(60):
            if m.docker('exec',core,'pg_isready','-U','fixture','-d','postgres',allow_failure=True).returncode==0:break
            time.sleep(.5)
        else:raise ValueError('isolated clone not ready')
        source=(CONTROL/'build-runtime/apps/platform/internal/lifecycle/infrastructure/postgres_repository.go').read_text(encoding='utf8')
        start=source.index('SELECT c.id,c.workspace_id,c.dataset_version_id');query=source[start:source.index(chr(96),start)]
        m.need('identity_kind' not in query,'R must be the reviewed single-schema repository')
        m.raw(OUT/'R-repository-query.sql',query.encode())
        for i,g in enumerate(old['Groups']):
            folder=OUT/g['Group'];folder.mkdir();db='runtime69_'+str(i)
            m.docker('exec',core,'createdb','-U','fixture',db)
            m.docker('exec',core,'pg_restore','-U','fixture','-d',db,'--exit-on-error','--no-owner','--no-acl','/run/'+g['Group']+'.dump')
            before=m.projected(core,db,m.columns(core,db),json.loads(m.q(core,db,'SELECT jsonb_agg(version ORDER BY version)::text FROM schema_migration;').stdout))
            for database,want in [(db,'ALLOW'),('rehearsal'+str(i),'DENY')]:
                tested=m.docker('exec','-e','D1_SCHEMA_TEST_DSN=postgres://fixture@127.0.0.1:5432/'+database+'?sslmode=disable','-e','D1_SCHEMA_EXPECT='+want,core,'/run/tools/fixture-schema.test','-test.v','-test.run','^TestRemainingFixtureSchemaNativePostgres$')
                m.raw(folder/(want+'-schema-gate.stdout.txt'),tested.stdout);m.raw(folder/(want+'-schema-gate.stderr.txt'),tested.stderr)
            copies=[cp for cp in g['Copies'] if cp['Key'] in ('fixture/p2.csv','fixture/t1.csv','fixture/t2.csv','fixture/h2-a.csv','fixture/m1-a.csv','fixture/m1-b.csv')]
            for n,cp in enumerate(copies):
                probe='PREPARE runtime_repository(uuid,uuid[]) AS '+query+";\nEXECUTE runtime_repository('"+cp['WorkspaceID']+"',ARRAY['"+cp['ID']+"']::uuid[]);"
                queried=m.q(core,db,probe);m.raw(folder/('copy-'+str(n)+'.stdout.txt'),queried.stdout)
                m.need(queried.stdout.count(cp['ID'].encode())==1,'exact R repository copy result differs')
            after=m.projected(core,db,m.columns(core,db),json.loads(m.q(core,db,'SELECT jsonb_agg(version ORDER BY version)::text FROM schema_migration;').stdout))
            m.need(before==after,'read-only R clone rehearsal changed old facts')
            m.save(folder/'old-facts-before.json',before);m.save(folder/'old-facts-after.json',after)
            result['groups'].append(dict(group=g['Group'],schema69_allow=True,protected71_rejected=True,exact_copies=len(copies),original_relations_unchanged=len(before)))
        m.need(sum(g['exact_copies'] for g in result['groups'])==6,'all six exact repository paths required')
    except BaseException as error:
        failure=dict(type=type(error).__name__,message=str(error));raise
    finally:
        m.docker('stop','--time','15',core,allow_failure=True)
        closed=m.closed(m.inspect(core));sources_closed=all(m.closed(m.inspect(cid)) for cid in originals)
        result.update(status='PASS_ISOLATED_R_P_ARTIFACT_AND_SCHEMA_CONTRACT' if failure is None and closed and sources_closed else 'FAILED_OR_UNCONFIRMED_RETAINED',failure=failure,clone_closed=closed,sources_remained_closed=sources_closed)
        m.save(OUT/'terminal.json',result)

if __name__=='__main__':main()
