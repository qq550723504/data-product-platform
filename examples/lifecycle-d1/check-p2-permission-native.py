#!/usr/bin/env python3
"""Isolated native permission regression; never reopens a retained fixture.

The pinned server's X-Minio-Source-Replication-Check returns before object
mutation. The Go test signs and guards EVERY DELETE with that check header.
Resources and evidence are retained, and only this run's two CIDs are stopped.
"""
import argparse, hashlib, json, os, pathlib, secrets, subprocess, uuid

ROOT = pathlib.Path(__file__).resolve().parents[2]
IMAGE = 'sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
SOURCE_NATIVE = 'f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce'
MODULE = 'v0.0.0-20251015172955-9e49d5e7a648'
MOD = pathlib.Path(os.environ['USERPROFILE'])/('go/pkg/mod/github.com/minio/minio@'+MODULE)

def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def need(ok, message):
    if not ok: raise RuntimeError(message)
def save(p, value):
    with p.open('x', encoding='utf-8', newline='\n') as f: json.dump(value, f, ensure_ascii=False, indent=2); f.write('\n')
def run(args, **kw):
    p=subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kw)
    if p.returncode: raise RuntimeError(f'{args[0]} command failed ({p.returncode}): '+p.stderr.decode('utf-8','strict')[:1500])
    return p.stdout

def main():
    ap=argparse.ArgumentParser(); ap.add_argument('--output',required=True,type=pathlib.Path); ap.add_argument('--retained-window',required=True,type=pathlib.Path); a=ap.parse_args()
    out=a.output.resolve(); out.mkdir(parents=True,exist_ok=False)
    private=pathlib.Path(os.environ['USERPROFILE'])/'.codex/fixture-private/p2-denial-regression'/str(uuid.uuid4());private.mkdir(parents=True,exist_ok=False)
    cids=[]; failure=None; succeeded=False
    retained=json.loads((a.retained_window/'post-seal-readback.json').read_text(encoding='utf-8'))
    ids=[r['cid'] for r in retained['owners']]
    def retained_state():
        rows=[]
        for i in range(0,len(ids),50):
            for r in json.loads(run(['docker','inspect',*ids[i:i+50]])):
                need(not r['State']['Running'] and r['State']['Pid']==0 and not r['HostConfig']['PortBindings'] and r['HostConfig']['RestartPolicy']['Name']=='no','retained fixture is not sealed')
                rows.append({'cid':r['Id'],'PID':r['State']['Pid'],'running':r['State']['Running']})
        return rows
    files={str(p):sha(p) for p in a.retained_window.rglob('*') if p.is_file()}
    # Private budget archives include the two actually consumed P2 slots.
    control=pathlib.Path(os.environ['USERPROFILE'])/'.codex/fixture-private/remaining-candidate18/authorized-execution01'
    budgets={str(p):sha(p) for p in control.glob('window01-*-budget.tar')}
    save(out/'retained-before.json',{'owners':retained_state(),'window_files':len(files),'used_budget_archives':budgets})
    try:
        # Prove the exact executable's module identity and the native short-circuit
        # contract before allowing the probe binary to run.
        run(['docker','cp',SOURCE_NATIVE+':/usr/local/bin/minio',str(private/'minio')])
        info=run(['go','version','-m',str(private/'minio')]).decode('utf-8','strict')
        (out/'native-buildinfo.txt').write_text(info,encoding='utf-8')
        need('github.com/minio/minio\t'+MODULE in info,'native module is not the pinned release')
        handlers=(MOD/'cmd/object-handlers.go').read_text(encoding='utf-8')
        delete=handlers[handlers.index('func (api objectAPIHandlers) DeleteObjectHandler'):]
        need(delete.index('policy.DeleteObjectAction')<delete.index('MinIOSourceReplicationCheck')<delete.index('opts, err := delOpts')<delete.index('deleteObject(ctx'),'native permission check is not before mutation')
        save(out/'native-contract.json',{'image':IMAGE,'binary_sha256':sha(private/'minio'),'module':MODULE,'handler_sha256':sha(MOD/'cmd/object-handlers.go'),'auth_sha256':sha(MOD/'cmd/auth-handler.go'),'mutation_guard':'X-Minio-Source-Replication-Check','ordinary_delete_disabled':True})
        env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
        built=subprocess.run(['go','test','-c','-o',str(private/'regression'),'./cmd/lifecycle-fixture-remaining'],cwd=ROOT/'apps/platform',env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        (out/'build.stdout.txt').write_bytes(built.stdout);(out/'build.stderr.txt').write_bytes(built.stderr);need(built.returncode==0,'native test build failed')
        nonce=uuid.uuid4().hex;name='dpp307-p2-probe-'+nonce
        access='root'+secrets.token_hex(8); secret=secrets.token_hex(24)
        envfile=private/'native.env';envfile.write_text('MINIO_ROOT_USER='+access+'\nMINIO_ROOT_PASSWORD='+secret+'\n',encoding='utf-8')
        copies={}
        for group in ('business-races','late-hold','mixed'):
            rows=json.loads((ROOT/f'apps/platform/cmd/lifecycle-fixture-reader/testdata/post-v22/{group}.json').read_text(encoding='utf-8'))
            copies[group]=[r['Copy'] for r in rows if r['Copy']['Key']!='fixture/p1.csv']
        save(private/'config.json',{'Endpoint':'127.0.0.1:9000','AccessKey':access,'SecretKey':secret,'Output':'/output','Copies':copies})
        volume=run(['docker','volume','create','--label','dpp.owner=p2-permission-regression',name+'-data']).decode().strip()
        server=run(['docker','create','--name',name+'-native','--label','dpp.owner=p2-permission-regression','--network','none','--restart','no','--env-file',str(envfile),'--mount','type=volume,source='+volume+',target=/data',IMAGE,'server','/data','--address','127.0.0.1:9000','--console-address','127.0.0.1:9001']).decode().strip();cids.append(server)
        native_image=json.loads(run(['docker','inspect',server]))[0]['Image'];need(native_image==IMAGE,'wrong native image')
        runner=run(['docker','create','--name',name+'-test','--label','dpp.owner=p2-permission-regression','--network','container:'+server,'--restart','no','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges','--read-only','--tmpfs','/tmp:rw,nosuid,nodev,mode=1777','--mount',f'type=bind,source={private},target=/private,readonly','--mount',f'type=bind,source={out},target=/output','--env','TEST_LIFECYCLE_PERMISSION_CONFIG=/private/config.json','--entrypoint','/private/regression',IMAGE,'-test.v','-test.run','^TestNativeExactDeletePermissionWithoutObjectDeletion$','-test.timeout=120s']).decode().strip();cids.append(runner)
        save(out/'allocation.json',{'server':server,'runner':runner,'volume':volume,'image':IMAGE,'test_binary_sha256':sha(private/'regression'),'retained_mounts_used':0,'published_ports':0,'network':'none; runner shares only new native namespace'})
        run(['docker','start',server])
        p=subprocess.run(['docker','start','-a',runner],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        (out/'native.stdout.txt').write_bytes(p.stdout);(out/'native.stderr.txt').write_bytes(p.stderr)
        result=json.loads(run(['docker','inspect',runner]))[0]
        need(p.returncode==0 and result['State']['ExitCode']==0,'native regression failed; see retained stdout/stderr')
        need((out/'native-permission-result.json').is_file(),'native receipt absent')
        succeeded=True
    except Exception as ex: failure=str(ex)
    finally:
        closed=[]
        for cid in reversed(cids):
            p=subprocess.run(['docker','stop','-t','3',cid],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            row=json.loads(run(['docker','inspect',cid]))[0]
            closed.append({'cid':cid,'PID':row['State']['Pid'],'running':row['State']['Running'],'noports':not row['HostConfig']['PortBindings'],'restart':row['HostConfig']['RestartPolicy']['Name']})
        (out/'native-server.log').write_bytes(run(['docker','logs',cids[0]])) if cids else None
        need(all(sha(pathlib.Path(p))==h for p,h in files.items()),'original failed evidence changed')
        need(all(sha(pathlib.Path(p))==h for p,h in budgets.items()),'used budgets changed')
        save(out/'retained-after.json',{'owners':retained_state(),'original_window_files_unchanged':len(files),'used_budget_archives_unchanged':budgets})
        save(out/'terminal.json',{'status':'PASS_ISOLATED_NONDELETE_REGRESSION' if succeeded else 'FAILED_RETAINED','failure':failure,'closure':closed,'old_source_starts':0,'old_authorization_reused':False,'old_budget_writes':0,'pins_released':0,'actual_object_deletions':0,'resources_retained':True})
    if failure: raise RuntimeError(failure)

if __name__=='__main__':main()
