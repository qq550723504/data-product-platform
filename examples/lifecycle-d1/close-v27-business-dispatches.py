"""Finite v27 NOT_SENT maintenance; no candidate, sender or deletion authority.

Reuse existing physical owner contract, Command observer, append-only delta and
IAM owner. prepare only allocates stopped owners. run requires an independent
review bound to the exact manifest; one terminal forbids any automatic replay.
"""
import argparse
import base64
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import uuid
from urllib.parse import urlsplit

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'post-v27-business-closure03'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/post-v27-business-closure03'
HISTORY=Path('C:/Users/Henry/.codex/worktrees/d1-c17/data-product-platform/docs/acceptance/evidence/issue307-d1/delete-confirmed-v27-execution01/window01')
OLD=PRIVATE/'remaining-candidate17'
PACKET=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate17'
EXECUTION='6f40bca9-cfe2-4362-ad2b-4e38bbbd8453'
CANDIDATE='20d08f0a2d334e779b64f0c082515131c285d9fd005c6fbaa94294b147f38d42'
BINDINGS='77ce0be4d5dccd95a276a5b4c3f060689bea870f4f3caef1a827ef81154efa24'
MANIFEST='manifest.json'
PREVIOUS=ROOT/'docs/acceptance/evidence/issue307-d1/post-v27-business-closure02'
spec=importlib.util.spec_from_file_location('v22_closure_reuse',Path(__file__).with_name('close-old-business-dispatches.py'))
c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
c.CONTROL=CONTROL;c.PUBLIC=PUBLIC;c.p1.CONTROL=CONTROL
p=c.p1
load,sha,write,need,inspect,docker,closed,now=c.load,c.sha,c.write,c.need,c.inspect,c.docker,c.closed,c.now

def packet():
    need(sha(PACKET/'candidate.json')==CANDIDATE and sha(PACKET/'bindings.json')==BINDINGS,'failed packet drift')
    need(sha(HISTORY/'terminal.json')=='a98a6016e64c71e4f883ac4dbeff975f794f0135d6aa9a80cc946a4e51a45672','original terminal drift')
    return load(PACKET/'candidate.json'),load(PACKET/'bindings.json')

def targets():
    rows=load(ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/core-audit01/actual-v27-targets.json')
    return [dict(Case=name,**{k:row[k] for k in ('Plan','Copy','Dispatch','Attempt','Approval','PlanHash')}) for name,row in zip(('P2','T1','T2'),rows)]

def inspect_many(ids):
    # Fixed full CIDs are 65 argv bytes each; bounded batches stay below the
    # Windows CreateProcess limit. Validate the complete union after every read.
    need(len(ids)==len(set(ids)) and all(len(cid)==64 and all(c in '0123456789abcdef' for c in cid) for cid in ids),'invalid owner set')
    result=[]
    for start in range(0,len(ids),64):result.extend(json.loads(docker('inspect',*ids[start:start+64])))
    need(len(result)==len(ids) and {x['Id'] for x in result}==set(ids),'physical owner lookup substituted')
    return result

def retained(active_core=None):
    history=load(HISTORY/'postfailure-closed-readback.json')
    ids=[x['cid'] for x in history['physical_owners']]
    need(len(ids)==len(set(ids))==540,'retained owner membership differs')
    actual=inspect_many(ids)
    descriptors={}
    budgets={}
    for x in actual:
        need(closed(x) or x['Id']==active_core and x['State']['Running'] and x['State']['Pid']>0,'retained owner not closed')
        descriptors[x['Id']]=hashlib.sha256(json.dumps({k:x[k] for k in ('Image','Config','HostConfig','Mounts')},sort_keys=True).encode()).hexdigest()
        for mount in x['Mounts']:
            if mount['Type']=='volume' and mount['Destination']=='/run/budget':budgets.setdefault(mount['Name'],x['Id'])
    budget_hashes={name:hashlib.sha256(docker('cp',cid+':/run/budget','-')).hexdigest() for name,cid in sorted(budgets.items())}
    files={}
    for folder in PRIVATE.glob('remaining-*'):
        if not folder.is_dir():continue
        paths=list(folder.rglob('permission.json'))+list(folder.glob('*/case-*/remaining.json'))+list(folder.glob('*/recover-*/recovery.json'))+list(folder.glob('*/checkpoint/*'))
        for path in paths:
            if path.is_file():files[path.relative_to(PRIVATE).as_posix()]=sha(path)
    need(len(budget_hashes)>=3 and len(files)>=30,'retained budgets/profiles/backups incomplete')
    prior_control=PRIVATE/'post-v27-business-closure02'
    need(sha(prior_control/'terminal.json')=='79ac5ce594abc3757df840de524ccf092e3ac5f799b8f94204f9131a6dfb47bb','partial maintenance terminal drift')
    for owner in load(prior_control/'manifest-source02.json')['owners'].values():need(closed(inspect(owner['cid'])),'prior maintenance owner active')
    need(load(prior_control/'worker-private/closure.json')['Enabled'] is False,'failed maintenance profile reactivated')
    prior_files={x.relative_to(prior_control).as_posix():sha(x) for x in prior_control.rglob('*') if x.is_file() and x.name!='host-key.private'}
    return dict(descriptors=descriptors,budgets=budget_hashes,files=files,history_readback_sha256=sha(HISTORY/'postfailure-closed-readback.json'),prior_maintenance_files=prior_files)

def physical(candidate,bindings,active_core=None):
    owners={}
    never_started=load(HISTORY/'postfailure-closed-readback.json')['six_senders_and_three_wire_never_started']
    need(len(never_started)==9,'no-send owner history incomplete')
    for fact in never_started:
        x=inspect(fact['cid']);need(closed(x) and x['State']['StartedAt']==fact['started_at']=='0001-01-01T00:00:00Z','retired sender/wire has started since failure')
    for g,b in zip(candidate['Groups'],bindings):
        need(g['Group']==b['group'],'group order drift')
        folder=OLD/g['Group']
        for path in folder.rglob('permission.json'):need(load(path)['Enabled'] is False,'old permission re-enabled')
        for path in folder.glob('case-*/remaining.json'):need(load(path)['Base']['Enabled'] is False,'old case re-enabled')
        for path in folder.glob('recover-*/recovery.json'):need(load(path)['Enabled'] is False,'old recovery re-enabled')
        gateway=inspect(g['Gateway']);mount=next(m for m in gateway['Mounts'] if m['Destination']=='/etc/caddy/Caddyfile')
        need(closed(gateway) and mount['Type']=='bind' and mount['RW'] is False and sha(p.hostpath(mount['Source']))==g['SealedGatewaySHA256'],'gateway not sealed/stopped')
        roles=[(cid,'/run/operator',candidate['SenderBinarySHA256']) for cid in g['Helpers'].values()]
        roles += [(cid,'/run/reader',candidate['ReaderBinarySHA256']) for cid in g['CaseHelpers'].values()]
        roles += [(r['container'],'/run/owner',candidate['OwnerBinarySHA256']) for r in b['root_owners'].values()]
        for cid,dest,digest in roles:
            x=inspect(cid);need(closed(x) and x['Image']==p.PG,'old execution owner not physically closed')
            mount=next(m for m in x['Mounts'] if m['Destination']==dest)
            need(mount['Type']=='bind' and mount['RW'] is False and sha(p.hostpath(mount['Source']))==digest,'old binary mount drift')
            owners[cid]=digest
    need(len(owners)==36,'v27 execution owner membership differs')
    g=candidate['Groups'][0];network=json.loads(docker('network','inspect',g['Network']))[0]
    need(network['Internal'] and set(network.get('Containers',{}))==({active_core} if active_core else set()),'closure network has unexpected active owner')
    return owners

def allocate(kind,binary,private,output,source,audit_id):
    args=['post-v27-close'] if kind=='observer' else ['-action',kind]
    need(kind in ('observer','check-disabled','disable-all'),'no enable action exists')
    output.mkdir()
    cid=p.text('create','--pull','never','--name','dpp307-v27-closure-'+audit_id[:8]+'-'+kind,'--label','lifecycle307.closure='+audit_id,
        '--network','container:'+source,'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--restart','no','--pids-limit','64','--memory','256m','--cpus','1',
        '--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=bind,source='+str(private)+',target=/run/private,readonly',
        '--mount','type=bind,source='+str(binary)+',target=/run/executor,readonly','--mount','type=bind,source='+str(output)+',target=/run/output','--entrypoint','/run/executor',p.PG,*args)
    return dict(kind=kind,cid=cid,binary=str(binary),binary_sha256=sha(binary),private=str(private),output=str(output),network_source=source,arguments=args)

def source_files():
    names=p.call(['git','-c','core.longpaths=true','ls-files','apps/platform','examples/lifecycle-d1']).decode().splitlines()
    return {n:sha(ROOT/n) for n in names if n.endswith(('.go','.py')) or n.endswith(('go.mod','go.sum'))}

def core_binding(base,g):
    u=urlsplit(base['PostgresDSN']);x=inspect(g['Core']);host='dpp-lifecycle307-808a6960-postgres'
    need(u.scheme=='postgres' and u.hostname==host==x['Name'].lstrip('/') and u.port==5432 and u.path=='/lifecycle307' and u.username=='fixture' and bool(u.password) and u.query=='sslmode=disable' and not u.fragment,'finite Core connection differs')
    need(base['WorkspaceID']==g['Copies'][0]['WorkspaceID'] and base['ActorID']==g['Actor'] and base['ProviderInstanceID']==g['Copies'][0]['ProviderInstanceID'] and base['IncarnationID']==g['Copies'][0]['IncarnationID'],'Core principal/provider binding differs')
    need(len(x['NetworkSettings']['Networks'])==1,'Core network expansion')
    endpoint=next(iter(x['NetworkSettings']['Networks'].values()))
    need(endpoint['NetworkID']==g['Network'] and host in (endpoint.get('Aliases') or [])+(endpoint.get('DNSNames') or []),'Core DNS/network binding differs')

def prepare():
    need(not p.call(['git','-c','core.longpaths=true','diff','HEAD','--name-only']).strip(),'commit reviewed implementation first')
    need(not CONTROL.exists() and not PUBLIC.exists(),'one-shot closure already allocated')
    rehearsal=ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/restored-core02/terminal.json'
    proof=load(rehearsal)
    need(proof['status']=='PASS_ACTUAL_RESTORED_SCHEMA_COMMAND_AND_REPLAY' and proof['source_retained_unchanged'] is True,'actual-schema complete Command/replay gate missing')
    need(sha(PREVIOUS/'final-Core-fingerprint.json')=='9a320170f3e583725239b86645e4dc976ffe635cd630200fbb5a7765148bba1f'
         and sha(PREVIOUS/'final-Core-rows.json')=='3603f0224a34d81efc67b7faeb052efb59b46767914903d120d79a30b8399214','recorded partial baseline drift')
    CONTROL.mkdir();PUBLIC.mkdir(parents=True)
    candidate,bindings=packet();before=retained();owners=physical(candidate,bindings);g=candidate['Groups'][0]
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(CONTROL/'observer'),'./cmd/lifecycle-fixture-old-dispatch'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    (CONTROL/'build.private').write_bytes(result.stdout+result.stderr);need(result.returncode==0,'observer build failed')
    need(sha(CONTROL/'observer')==proof['observer_binary_sha256'],'observer differs from actual-schema accepted binary')
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat,PrivateFormat,NoEncryption
    key=Ed25519PrivateKey.generate();(CONTROL/'host-key.private').write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
    public=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex();audit_id=str(uuid.uuid4())
    worker=CONTROL/'worker-private';worker.mkdir();root=CONTROL/'root-private'
    shutil.copytree(OLD/'business-races/root-check-disabled',root)
    base=load(OLD/'business-races/case-business/remaining.json')['Base'];core_binding(base,g)
    cfg=dict(Enabled=False,AuditID=audit_id,WorkspaceID=base['WorkspaceID'],ActorID=base['ActorID'],Purpose='POST_V27_BUSINESS_NOT_SENT_CLOSURE',PostgresDSN=base['PostgresDSN'],Core=g['Core'],BinarySHA256=sha(CONTROL/'observer'),PublicKey=public,EvidenceSHA256='0'*64,Token=secrets.token_hex(32),Owners=owners,NotBefore='0001-01-01T00:00:00Z',ExpiresAt='0001-01-01T00:00:00Z',MaxProviderDeletes=0)
    write(worker/'closure.json',cfg);write(CONTROL/'closure-disabled-original.json',cfg)
    allocated={'observer':allocate('observer',CONTROL/'observer',worker,CONTROL/'output-observer',g['Core'],audit_id)}
    for action in ('check-disabled','disable-all'):allocated[action]=allocate(action,OLD/'owner',root,CONTROL/('output-'+action),g['Native'],audit_id)
    manifest=dict(status='PREPARED_DISABLED_V27_NONDELETE_CLOSURE_ONLY',audit_id=audit_id,source_head=p.call(['git','rev-parse','HEAD']).decode().strip(),source_files=source_files(),source=g,retained=before,targets=targets(),execution_owners=owners,owners=allocated,public_key=public,
        activation_fields=['Enabled','NotBefore','ExpiresAt','EvidenceSHA256'],inputs={x.relative_to(CONTROL).as_posix():sha(x) for x in CONTROL.rglob('*') if x.is_file() and x.name!='host-key.private' and x.suffix!='.private'},max_provider_deletes=0,pins_released=0,budgets_refunded=0,
        actual_schema_rehearsal_sha256=sha(rehearsal),partial_baseline_fingerprint_sha256=sha(PREVIOUS/'final-Core-fingerprint.json'),revoked_baseline={'P2':True,'T1':False,'T2':False})
    write(CONTROL/'manifest.json',manifest);write(PUBLIC/'preparation.json',manifest)
    for owner in allocated.values():need(closed(c.own_contract(owner,manifest)),'allocated owner not stopped')
    print(json.dumps(dict(status=manifest['status'],manifest_sha256=sha(CONTROL/'manifest.json'),source_started=False)))

def fresh_iam(owner):
    folder=Path(owner['output']);result=load(folder/'result.json');users=set(load(CONTROL/'root-private/iam.json')['disabled_identities'])
    need(len(users)==13 and result['action']=='check-disabled' and result['exact_policy_set_verified'] is True and not result['changed'] and set(result['before'])==users and set(result['before'].values())=={'disabled'},'fresh complete IAM proof missing')
    x=inspect(owner['cid']);begin,end=(dt.datetime.fromisoformat(x['State'][k]) for k in ('StartedAt','FinishedAt'))
    need(closed(x) and x['State']['ExitCode']==0 and dt.timedelta()<=now()-end<=dt.timedelta(seconds=45),'IAM readback stale')
    records=[];user_reads=[];policy_reads=0;absent='d4e524c04808a6960'
    for start in sorted(folder.glob('start-*.json')):
        request=load(start);observation=load(folder/start.name.replace('start-','observation-'))
        need(request['method']=='GET' and request['path'] in ('/minio/admin/v3/user-info','/minio/admin/v3/info-canned-policy') and not observation['transport_error'],'non-read-only or failed IAM request')
        if request['path']=='/minio/admin/v3/user-info':
            user=request['access'];need(user in users|{absent},'foreign IAM identity read')
            need(observation['http_status']==(404 if user==absent else 200),'IAM success/absence status differs')
            user_reads.append(user)
        else:
            need(observation['http_status']==200,'IAM policy read failed');policy_reads+=1
        need(begin<=dt.datetime.fromisoformat(request['at'])<=dt.datetime.fromisoformat(observation['at'])<=end,'IAM request/observation time differs')
        records.append(dict(start_sha256=sha(start),observation_sha256=sha(folder/start.name.replace('start-','observation-'))))
    # Frozen root owner verifies the absent user's official SDK error code
    # XMinioAdminNoSuchUser before exit0/result; 404 alone is not that proof.
    need(len(records)==27 and len(user_reads)==len(set(user_reads))==14 and set(user_reads)==users|{absent} and policy_reads==13,'IAM request inventory differs')
    return dict(result=result,finished_at=end.isoformat(),technical_requests=records,quantity=len(records),amount='NOT_ASSESSED',absent_identity=absent,absence_code_verified_by_frozen_owner='XMinioAdminNoSuchUser')


def frozen(manifest):
    packet();need(p.call(['git','rev-parse','HEAD']).decode().strip()==manifest['source_head'] and source_files()==manifest['source_files'],'implementation source drift')
    for name,digest in manifest['inputs'].items():
        if name!='worker-private/closure.json':need(sha(CONTROL/name)==digest,'immutable closure input drift')

def current_rows(native,core):return [json.loads(x) for x in native.sql(core,c.audit.query()).splitlines()]

def seal(manifest,started,native,before,anchors,result):
    # Every close is independent. A missing profile or unknown observer result
    # cannot skip the physical stops or erase a partially committed Core fact.
    result['profile_disabled']=False;g=manifest['source']
    try:
        cfg=load(CONTROL/'worker-private/closure.json');cfg['Enabled']=False
        (CONTROL/'worker-private/closure.json').write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8',newline='\n')
        result['profile_disabled']=load(CONTROL/'worker-private/closure.json')['Enabled'] is False
    except Exception:pass
    observer=manifest['owners']['observer']['cid']
    try:docker('stop','-t','15',observer)
    except Exception:pass
    result['final_Core']='UNCONFIRMED'
    try:
        need(closed(inspect(observer)) and inspect(g['Core'])['State']['Running'],'no stable Core/observer boundary')
        fp=native.fingerprints(g['Core']);write(PUBLIC/'final-Core-fingerprint.json',fp)
        write(PUBLIC/'final-Core-rows.json',current_rows(native,g['Core']))
        result['final_delta']=c.delta(before,fp,anchors,p.append_anchors(native,g['Core']),complete=False)
        result['final_Core']='OBSERVED_AFTER_OBSERVER_STOPPED'
    except Exception:pass
    result['identity_containment']='UNCHANGED_NO_ENABLE_PATH'
    try:
        if g['Native'] in started and inspect(g['Native'])['State']['Running']:
            result['identity_containment']='UNCONFIRMED'
            c.run_owner(manifest,'disable-all')
            status=load(Path(manifest['owners']['disable-all']['output'])/'status-changes.json')
            users=set(load(CONTROL/'root-private/iam.json')['disabled_identities'])
            need(not status['unconfirmed'] and set(status['disabled_readback'])==users and set(status['disabled_readback'].values())=={'disabled'},'disable-all not confirmed')
            result['identity_containment']='FRESH_DISABLED_READBACK'
    except Exception:result['identity_containment']='UNCONFIRMED'
    close=[]
    for cid in [x['cid'] for x in manifest['owners'].values()]+list(reversed(started)):
        item=dict(cid=cid)
        try:docker('stop','-t','15',cid)
        except Exception:item['stop_error']=True
        try:item['stopped_PID0']=closed(inspect(cid))
        except Exception:item['stopped_PID0']=False
        close.append(item)
    result['closure']=close
    try:result['retained_unchanged']=retained()==manifest['retained']
    except Exception:result['retained_unchanged']=False

def run(review,review_sha):
    manifest=load(CONTROL/MANIFEST);report=load(review)
    need(sha(review)==review_sha and report['status']=='PASS_BOUNDED_INDEPENDENT_V27_CLOSURE_MANIFEST_REVIEW' and report['remaining_blockers']==[] and report['manifest_sha256']==sha(CONTROL/MANIFEST),'independent exact-manifest review missing')
    need(not (CONTROL/'terminal.json').exists() and not (PUBLIC/'terminal.json').exists(),'one-shot already terminal; never replay')
    frozen(manifest);need(retained()==manifest['retained'],'retained state drift');c.profile(manifest,False)
    candidate,bindings=packet();need(physical(candidate,bindings)==manifest['execution_owners'],'owner map drift')
    g=manifest['source'];started=[];failure=None;before=anchors=None;native=p.native_module();result=dict(native_delete=0,pins_released=0,budgets_refunded=0)
    try:
        for cid in (g['Native'],g['Core']):started.append(cid);docker('start',cid)
        native.wait_pg(g['Core'],'fixture');before=native.fingerprints(g['Core']);need(before==load(PREVIOUS/'final-Core-fingerprint.json') and sha(PREVIOUS/'final-Core-fingerprint.json')==manifest['partial_baseline_fingerprint_sha256'],'explicit partial baseline drift')
        p.ALLOWED_RELATIONS=c.ALLOWED;anchors=p.append_anchors(native,g['Core'])
        write(PUBLIC/'Core-before.json',before);write(PUBLIC/'Core-before-rows.json',current_rows(native,g['Core']))
        for target,row in zip(manifest['targets'],load(ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/core-audit01/actual-v27-targets.json')):need(target['Attempt']==row['Attempt'],'audited target drift')
        c.run_owner(manifest,'check-disabled');iam=fresh_iam(manifest['owners']['check-disabled']);docker('stop','-t','15',g['Native']);need(closed(inspect(g['Native'])),'Native not stopped')
        need(physical(candidate,bindings,g['Core'])==manifest['execution_owners'],'live owner map drift')
        # The expensive budget/backup check is performed before IAM; physical
        # signing is refreshed after it. No selected/retired sender was started.
        ids=[x['cid'] for x in load(HISTORY/'postfailure-closed-readback.json')['physical_owners']]
        state=inspect_many(ids);need(all(closed(x) or x['Id']==g['Core'] for x in state),'retained sender/source active')
        need(now()-dt.datetime.fromisoformat(iam['finished_at'])<=dt.timedelta(seconds=45),'fresh IAM evidence expired')
        at=now();bundle=dict(manifest_sha256=sha(CONTROL/MANIFEST),at=at.isoformat(),IAM=iam,physically_checked_owners=540,execution_owners=manifest['execution_owners'],Core_before_sha256=sha(PUBLIC/'Core-before.json'))
        write(PUBLIC/'trusted-evidence.json',bundle)
        cfg=load(CONTROL/'closure-disabled-original.json');cfg.update(Enabled=True,NotBefore=at.isoformat(),ExpiresAt=(at+dt.timedelta(minutes=40)).isoformat(),EvidenceSHA256=sha(PUBLIC/'trusted-evidence.json'))
        (CONTROL/'worker-private/closure.json').write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8',newline='\n')
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        key=Ed25519PrivateKey.from_private_bytes((CONTROL/'host-key.private').read_bytes())
        body=dict(AuditID=manifest['audit_id'],Nonce=str(uuid.uuid4()),At=at.isoformat(),Execution=EXECUTION,CandidateSHA256=CANDIDATE,BindingsSHA256=BINDINGS,Core=g['Core'],EvidenceSHA256=cfg['EvidenceSHA256'],Targets=manifest['targets'],Owners=cfg['Owners'],StoppedPIDZero=True,NoRestart=True,ConfigDisabled=True,DeleteIdentityDisabled=True,GatewaySealed=True)
        raw=json.dumps(body,separators=(',',':')).encode();write(CONTROL/'worker-private/proof.json',dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()))
        frozen(manifest);c.profile(manifest,True);c.run_owner(manifest,'observer')
        observed=load(CONTROL/'output-observer/result.json');need(observed['completed'] and len(observed['cases'])==3,'observer incomplete');result['observer']=observed
        after=native.fingerprints(g['Core']);write(PUBLIC/'Core-after.json',after);write(PUBLIC/'Core-after-rows.json',current_rows(native,g['Core']))
        result['delta']=c.delta(before,after,anchors,p.append_anchors(native,g['Core']),complete=False)
        need(result['delta']['new_row_counts']==dict(lifecycle_approval_revocation=2,lifecycle_provider_observation=3,lifecycle_dispatch_release=3,command_idempotency=5,audit_event=8,evidence=8,evidence_relation=8,outbox_event=8),'exact partial-baseline completion counts differ')
    except Exception as e:failure=type(e).__name__+': '+str(e)
    finally:
        seal(manifest,started,native,before,anchors,result);result['failure']=failure
        result['status']='PASS_V27_THREE_NOT_SENT_APPEND_ONLY_SEALED' if not failure and result['retained_unchanged'] and result['profile_disabled'] and result['final_Core']=='OBSERVED_AFTER_OBSERVER_STOPPED' and result['identity_containment']!='UNCONFIRMED' and all(x['stopped_PID0'] for x in result['closure']) else 'FAILED_OR_UNCONFIRMED_KEEP_ALL_FACTS_NO_REPLAY'
        write(PUBLIC/'terminal.json',result);write(CONTROL/'terminal.json',result)
    print(json.dumps(dict(status=result['status'],failure=failure,native_delete=0)))
    return failure is None and result['status']=='PASS_V27_THREE_NOT_SENT_APPEND_ONLY_SEALED'

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=('prepare','run'));parser.add_argument('--review',type=Path);parser.add_argument('--review-sha256');args=parser.parse_args()
    if args.action=='prepare':prepare()
    else:
        need(args.review is not None and args.review_sha256 is not None,'exact independent manifest review required')
        raise SystemExit(0 if run(args.review,args.review_sha256) else 1)
