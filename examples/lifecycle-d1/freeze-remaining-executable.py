"""Freeze the current candidate after complete native NON-DELETE acceptance.

No arming, human-confirmation creation, DELETE, cleanup or merge mode exists.
Fresh source verification is GET/HEAD-only plus read-only Core fingerprints.
Pre-created case/sender/recovery owners are retained with send permission false.
"""
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
import sys
import time

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('runner',Path(__file__).with_name('prepare-remaining-runner.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
spec=importlib.util.spec_from_file_location('current_window',Path(__file__).with_name('remaining_window.py'))
window=importlib.util.module_from_spec(spec);spec.loader.exec_module(window)
WINDOW=window.compiled()
EXECUTION=WINDOW['Execution'];PROFILE=WINDOW['Profile']
START=WINDOW['NotBefore'];END=WINDOW['NotAfter'];RECOVERY_END=WINDOW['RecoveryEnd']
GENERATION=PROFILE.split('/')[0]
SGT=dt.timezone(dt.timedelta(hours=8))
WINDOW_LABEL=window.instant(START).astimezone(SGT).strftime('%Y-%m-%d %H:%M')+'–'+window.instant(END).astimezone(SGT).strftime('%H:%M')
PRIVATE=r.PRIVATE/('remaining-'+GENERATION)
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1'/('remaining-'+GENERATION)
PREP=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-executable03'
KEYS={'fixture/p1.csv':'P1','fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2','fixture/h2-a.csv':'H2','fixture/m1-a.csv':'M1A','fixture/m1-b.csv':'M1B'}
COPY_FIELDS=('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')

def compact(v):return json.dumps(v,separators=(',',':'),ensure_ascii=False).encode()
def digest(raw):return hashlib.sha256(raw).hexdigest()
def cp_order(v):return {k:v[k] for k in COPY_FIELDS}

def docker_logs(ident):
    result=subprocess.run(['docker','logs',ident],capture_output=True)
    r.require(result.returncode==0,'Cannot read retained owner logs')
    return result.stdout+result.stderr

def close_known(identifiers):
    rows=[]
    for ident in dict.fromkeys(reversed(identifiers)):
        try:
            state=r.inspect(ident)
            if state['State']['Running']:r.docker('stop','--time','20',ident)
            state=r.inspect(ident)
            closed=(not state['State']['Running'] and state['State']['Pid']==0
                    and state['HostConfig']['RestartPolicy']['Name']=='no'
                    and not state['HostConfig'].get('PortBindings')
                    and not any(state['NetworkSettings']['Ports'].values()))
            rows.append({'id':ident,'stopped_pid_zero':closed})
        except Exception:
            rows.append({'id':ident,'stopped_pid_zero':False})
    return {'containers':rows,'all_closed':all(x['stopped_pid_zero'] for x in rows),
            'retained':True,'native_delete':0,'cleanup':0}


def terminal_owners(packet, private, terminal_path, expected_status, verifier):
    terminal=r.load(terminal_path)
    r.require(terminal.get('status',terminal.get('state'))==expected_status
              and terminal['no_resume'] is True,'Prior execution is not terminal')
    closed_ids=[]
    for g,b in zip(r.load(packet/'candidate.json')['Groups'],r.load(packet/'bindings.json')):
        folder=private/g['Group']
        for stage in g['Helpers']:
            r.require(r.load(folder/stage/'permission.json')['Enabled'] is False,'Prior operator permission enabled')
            if stage.startswith('recover-'):r.require(r.load(folder/stage/'recovery.json')['Enabled'] is False,'Prior recovery not revoked')
        for stage in g['CaseHelpers']:r.require(r.load(folder/('case-'+stage)/'remaining.json')['Base']['Enabled'] is False,'Prior case not revoked')
        for action in b['root_owners']:r.require(r.load(folder/('root-'+action)/'permission.json')['Enabled'] is False,'Prior root permission enabled')
        ids=[*g['Helpers'].values(),*g['CaseHelpers'].values(),*[x['container'] for x in b['root_owners'].values()],g['Core'],g['Native'],g['Gateway'],g['CheckpointCore'],g['CheckpointNative'],b['recovery_redis']]
        for ident in ids:
            r.require(verifier.closed(r.inspect(ident)),'Prior failed owner active');closed_ids.append(ident)
    return {'status':'PASS_TERMINAL_PHYSICAL_CLOSURE','terminal_sha256':r.sha(terminal_path),
            'containers':closed_ids,'old_authority_reused':False,'native_delete':0}


def complete_frozen_preflight():
    """The frozen packet must pass real main, with every host gate, before indexing."""
    public=PUBLIC/'complete-entry'
    command=[sys.executable,'-B',str(ROOT/'examples/lifecycle-d1/verify-remaining-native.py'),
        '--packet',str(PUBLIC),'--candidate-sha256',r.sha(PUBLIC/'candidate.json'),
        '--bindings-sha256',r.sha(PUBLIC/'bindings.json'),
        '--private',str(PRIVATE/'complete-entry'),'--output',str(public),
        '--reader',str(PRIVATE/'reader'),'--checker',str(PRIVATE/'readonly-check.exe'),
        '--checker-sha256',r.sha(PRIVATE/'readonly-check.exe')]
    result=subprocess.run(command,cwd=ROOT,capture_output=True)
    (PRIVATE/'complete-entry-stdout.private').write_bytes(result.stdout)
    (PRIVATE/'complete-entry-stderr.private').write_bytes(result.stderr)
    r.write(PUBLIC/'complete-entry-invocation.json',{'exit_code':result.returncode,
        'entry_sha256':r.sha(ROOT/'examples/lifecycle-d1/verify-remaining-native.py'),
        'stdout_sha256':digest(result.stdout),'stderr_sha256':digest(result.stderr),'native_delete':0})
    r.require(result.returncode==0,'Complete frozen entry failed; retain generation and diagnose')
    receipt=r.load(public/'complete.json')
    r.require(receipt['status']=='PASS_COMPLETE_FRESH_NATIVE_PREFLIGHT'
              and receipt['candidate_sha256']==r.sha(PUBLIC/'candidate.json')
              and receipt['execution']==EXECUTION
              and receipt['host_gate']['status']=='PASS_COMPLETE_SEALED_HOST_GATE'
              and receipt['host_gate']['execution']==EXECUTION
              and receipt['host_gate']['permissions_false']==32
              and len(receipt['host_gate']['cases'])==7 and len(receipt['host_gate']['budgets'])==3
              and len(receipt['host_gate']['recoveries'])==7 and len(receipt['host_gate']['networks'])==3,
              'Complete frozen entry did not prove all host gates')

def main():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding,PrivateFormat,PublicFormat,NoEncryption
    r.require(not r.call(['git','status','--porcelain']).strip(),'Commit reviewed source and native receipts before freeze')
    r.require(not PRIVATE.exists() and not PUBLIC.exists(),'Frozen generation already allocated; never replace it')
    r.require(r.load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-executable04/checkpoint-completion.json')['C_protection']=='PASS_NATIVE_GOVERNANCE_BLOCK','Mixed continuation incomplete')
    PRIVATE.mkdir();PUBLIC.mkdir();source=r.call(['git','rev-parse','HEAD']).decode().strip()
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    for name,pkg in [('reader','lifecycle-fixture-reader'),('operator','lifecycle-fixture-remaining'),('owner','lifecycle-fixture-remaining-iam')]:
        result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(PRIVATE/name),'./cmd/'+pkg],cwd=ROOT/'apps/platform',env=env,capture_output=True)
        r.require(result.returncode==0,'Frozen '+name+' build failed')
        (PUBLIC/(name+'-modules.txt')).write_bytes(r.call(['go','version','-m',PRIVATE/name]))
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(PRIVATE/'readonly-check.exe'),'./cmd/lifecycle-fixture-readonly-check'],cwd=ROOT/'apps/platform',capture_output=True)
    r.require(result.returncode==0,'File-only semantic checker build failed')
    r.write(PUBLIC/'verification-build.json',{'source':source,'checker_sha256':r.sha(PRIVATE/'readonly-check.exe'),'entry_sha256':r.sha(ROOT/'examples/lifecycle-d1/verify-remaining-native.py'),'provider_io':0})
    verify_spec=importlib.util.spec_from_file_location('complete_native_preflight',ROOT/'examples/lifecycle-d1/verify-remaining-native.py')
    verifier=importlib.util.module_from_spec(verify_spec);verify_spec.loader.exec_module(verifier)
    old_packet=PUBLIC.parent/'remaining-candidate07'
    old_candidate=r.load(old_packet/'candidate.json')
    r.require(r.sha(old_packet/'candidate.json')=='b9660f17cbb745e781a06d50e8ec0e81c87e654318420533b75b4ada29728949'
              and r.sha(old_packet/'bindings.json')=='ede110b20b2e0815060a7ba1e599db073e56c4c476a3d92d358e20a6ec898562','Historical source packet drift')
    r.write(PUBLIC/'source-sealed-preconditions.json',verifier.retained_owners_gate(old_packet,r.PRIVATE/'remaining-candidate07'))
    prior_packet=PUBLIC.parent/'remaining-candidate08'
    r.require(r.sha(prior_packet/'candidate.json')=='730b92a761ca953453fb4914b5d63e1be2d6ce76bea806db7c0e9f5ba4a61b82'
              and r.sha(prior_packet/'bindings.json')=='88b39a6f25ffb0cc959d90bdfd50de7b8479a8d5dc345598e4583ab39a1f028f','Superseded preparation snapshot drift')
    r.write(PUBLIC/'prior-preparation-closed.json',verifier.retained_owners_gate(prior_packet,r.PRIVATE/'remaining-candidate08'))
    expired_packet=PUBLIC.parent/'remaining-candidate09'
    r.require(r.sha(expired_packet/'candidate.json')=='3a4a93c449a99f54925fa2924857b88e34929a4731b8e7bfdc3933d2eb29ef76'
              and r.sha(expired_packet/'bindings.json')=='985906a224f1e78899402b9d8af3daa93f37767bb73fb3e1be9edde61fec1ab8','Expired v19 preparation snapshot drift')
    r.write(PUBLIC/'expired-v19-closed.json',verifier.retained_owners_gate(expired_packet,r.PRIVATE/'remaining-candidate09'))
    # The failed v20 execution is terminal evidence, not a reusable execution
    # profile. Its legitimate seal revoked case/recovery Enabled fields, so do
    # not demand the old private profile hash as a current-generation contract.
    failed_packet=PUBLIC.parent/'remaining-candidate10';failed_private=r.PRIVATE/'remaining-candidate10'
    r.write(PUBLIC/'failed-v20-closed.json',terminal_owners(failed_packet,failed_private,
        failed_private/'authorized-execution01/terminal.json','FAILED_STOPPED_SEALED',verifier))
    failed_packet=PUBLIC.parent/'remaining-candidate11';failed_private=r.PRIVATE/'remaining-candidate11'
    r.write(PUBLIC/'failed-v21-closed.json',terminal_owners(failed_packet,failed_private,
        failed_private/'authorized-execution01/terminal-final.json',
        'FAILED_STOPPED_SEALED_WITH_POST_SEAL_IAM_UNCONFIRMED',verifier))
    files={}
    for name in r.call(['git','ls-files','apps/platform','migrations','examples/lifecycle-d1']).decode().splitlines():
        path=ROOT/name
        if path.suffix in {'.go','.py','.ps1','.sql','.Caddyfile','.mod','.sum','.yaml','.yml'} or (path.suffix=='.json' and name.startswith('apps/platform/')):
            raw=path.read_bytes();files[name]={'raw_sha256':digest(raw),'lf_sha256':digest(raw.replace(b'\r\n',b'\n'))}
    r.write(PUBLIC/'source-freeze.json',{'source_commit':source,'contract':'tracked source; raw and CRLF-to-LF hashes; embedded scope JSON included','files':files})
    scope=r.load(ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
    targets=[cp_order(cp) for s in scope for cp in s['Copies'] if cp['Key'] in KEYS]
    target_digest=digest(compact(targets))
    host_key=Ed25519PrivateKey.generate();wire_key=Ed25519PrivateKey.generate()
    for name,key in [('topology',host_key),('wire',wire_key)]:
        (PRIVATE/(name+'-key.private')).write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
    host_pub=host_key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()
    wire_pub=wire_key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
    wire_private=(wire_key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption())+wire_pub).hex()
    candidate={'Profile':PROFILE,'Execution':EXECUTION,'Source':source,'SourceFreezeSHA256':r.sha(PUBLIC/'source-freeze.json'),'ReaderBinarySHA256':r.sha(PRIVATE/'reader'),'SenderBinarySHA256':r.sha(PRIVATE/'operator'),'OwnerBinarySHA256':r.sha(PRIVATE/'owner'),'TargetDigest':target_digest,'PublicKey':host_pub,'WirePublicKey':wire_pub.hex(),'NotBefore':START,'NotAfter':END,'Groups':[]}
    bindings=[];all_helpers=[];source_gateways=[];owned=[];failure=None
    profile_windows={'cases':[],'recoveries':[]}
    try:
        for s in scope:
            group=s['Group'];project='dpp-lifecycle307-'+s['Run'][:8];original=r.PRIVATE/project;config=r.load(original/'config.json')
            prepared=PREP/group;history=r.load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-native01'/group/'manifest.json')
            checkpoint=r.load(prepared/'checkpoint.json') if group!='mixed' else r.load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-executable04/checkpoint-completion.json')
            private=PRIVATE/group;private.mkdir();public=PUBLIC/group;public.mkdir()
            pg,mi,gw=(r.inspect(project+'-'+name)['Id'] for name in ('postgres','minio','gateway'))
            network=json.loads(r.docker('network','inspect',project+'-private'))[0]
            r.require(network['Internal'] and network['Id']==history['environment']['network']['id'],'Source network drifted')
            for ident in (pg,mi,gw):
                state=r.inspect(ident);r.require(state['State']['Pid']==0 and state['HostConfig']['RestartPolicy']['Name']=='no' and not any(state['NetworkSettings']['Ports'].values()),'Source not sealed')
            backup=r.PRIVATE/'remaining-executable03'/group/'checkpoint'
            for filename,key in [('core.dump','core_dump_sha256'),('globals.sql','globals_sha256'),('native.tar','native_archive_sha256')]:r.require(r.sha(backup/filename)==checkpoint[key],'Backup changed')
            # Same complete entry as future pre-send verification; no second
            # host raw-SHA or HTTP-status gate is composed during the window.
            old_group=next(g for g in old_candidate['Groups'] if g['Group']==group)
            verifier.collect_group(old_packet,old_group,private/'fresh-readonly',public/'preflight',PRIVATE/'reader',PRIVATE/'readonly-check.exe')
            verify_out=public/'readonly';shutil.copytree(public/'preflight/readonly',verify_out)
            r.write(public/'core-readonly.json',r.load(public/'preflight/core-after.json'))
            owned.extend([pg,mi,r.load(public/'preflight/owner.json')['id']])
            old=r.PRIVATE/'remaining-executable03'/group/'source-verifier';vc=r.load(old/'remaining.json')
            native=r.load(verify_out/'native-verification.json');r.require(native['status']=='PASS_EXACT_NATIVE_READONLY','Native evidence incomplete')
            copies=[cp_order(o['Copy']) for o in native['copies']]
            iam={'disabled_identities':native['disabled_identities'],'policy_sha256':native['policy_sha256'],'policy_file_sha256':{name:r.sha(verify_out/('native-policy-'+name+'.json')) for name in native['policy_sha256']}};r.write(public/'iam.json',iam)
            heads=r.load(prepared/'policy-heads.json')
            selected_heads={cp['DatasetVersionID']:heads[cp['DatasetVersionID']] for cp in copies if group=='business-races' and cp['Key'] not in ('fixture/control-hold.csv','fixture/control-retention.csv')}
            case_base=window.case_profile(r.load(r.PRIVATE/'remaining-executable03'/group/'source-owner/remaining.json'),selected_heads,secrets.token_hex(32),vc['Base']['Epoch'])
            active=private/'gateway-active.Caddyfile'
            if group=='business-races':active.write_bytes(r.gateway(case_base['Base'],copies))
            else:active.write_text("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n @metadata {\n method GET HEAD\n expression {http.request.header.Authorization}.contains('Credential="+config['InspectAccess']+"/')\n }\n reverse_proxy @metadata minio:9000\n respond 403\n }\n}\n",encoding='utf-8')
            sealed=private/'gateway-sealed.Caddyfile';sealed.write_bytes((original/'Caddyfile').read_bytes())
            (public/active.name).write_bytes(active.read_bytes());(public/sealed.name).write_bytes(sealed.read_bytes())
            g={'Group':group,'Actor':config['ActorID'],'Core':pg,'Native':mi,'Gateway':gw,'Network':network['Id'],'CheckpointCore':checkpoint['restored_core'],'CheckpointNative':checkpoint['restored_native'],'CoreDumpSHA256':r.sha(backup/'core.dump'),'GlobalsSHA256':r.sha(backup/'globals.sql'),'NativeArchiveSHA256':r.sha(backup/'native.tar'),'IAMSHA256':r.sha(public/'iam.json'),'GatewaySHA256':r.sha(active),'SealedGatewaySHA256':r.sha(sealed),'Copies':copies,'Helpers':{},'ConfigSHA256':{},'CaseHelpers':{},'CaseConfigSHA256':{},'PolicyHeads':selected_heads}
            source_gateways.append(gw)
            # Actual durable Redis queue for future selected reconciliation.
            redis=project+'-'+GENERATION+'-recovery-redis';r.docker('volume','create','--label','lifecycle307.remaining='+EXECUTION,redis+'-data')
            rid=r.docker('create','--pull','never','--name',redis,'--label','lifecycle307.remaining='+EXECUTION,'--network',network['Id'],'--mount','type=volume,source='+redis+'-data,target=/data',r.REDIS,'redis-server','--appendonly','yes').decode().strip();owned.append(rid)
            budget=project+'-'+GENERATION+'-budget';r.docker('volume','create','--label','lifecycle307.remaining='+EXECUTION,budget)
            outputs=private/'outputs';outputs.mkdir()
            if group=='business-races':(outputs/'remaining-state.json').write_bytes((ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner02/business/remaining-state.json').read_bytes())
            modes=window.CASE_MODES[group]
            for stage,mode in modes.items():
                folder=private/('case-'+stage);folder.mkdir();cfg=json.loads(json.dumps(case_base))
                if stage=='close-H1':
                    closure=digest(compact({'execution':EXECUTION,'source':source,'reader':candidate['ReaderBinarySHA256'],'target':target_digest,'owner':g['CaseHelpers']['late']}))
                    cfg.update(ClosurePublicKey=host_pub,ClosureCandidateSHA256=closure,ClosureOwners={g['CaseHelpers']['late']:candidate['ReaderBinarySHA256']})
                r.write(folder/'remaining.json',cfg);(folder/'manifest.json').write_bytes((old/'manifest.json').read_bytes())
                ident=r.docker('create','--pull','never','--name',project+'-'+GENERATION+'-case-'+stage,'--label','lifecycle307.remaining='+EXECUTION,'--network','container:'+gw,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={ROOT}/industry-packs,target=/run/repo/industry-packs,readonly','--mount',f'type=bind,source={outputs},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip();owned.append(ident)
                g['CaseHelpers'][stage]=ident;g['CaseConfigSHA256'][stage]=r.sha(folder/'remaining.json')
                profile_windows['cases'].append(window.project_case(cfg,group,stage,g['CaseConfigSHA256'][stage]))
            stages=['wire']+[stage for cp in s['Copies'] if cp['Key'] in KEYS for stage in (KEYS[cp['Key']],'recover-'+KEYS[cp['Key']])]
            for stage in stages:
                folder=private/stage;folder.mkdir();output=outputs/stage;output.mkdir()
                f={'Group':group,'PostgresDSN':config['PostgresDSN'],'InspectAccess':config['InspectAccess'],'InspectSecret':config['InspectSecret'],'DeleteAccess':'delete'+s['Run'][:8],'DeleteSecret':config['Token'] if stage!='wire' and not stage.startswith('recover-') else '', 'Token':case_base['Base']['Token'],'WirePrivateKey':wire_private if stage=='wire' else '', 'RedisAddr':redis+':6379'}
                r.write(folder/'operator.json',f)
                if stage.startswith('recover-'):
                    rc={k:config[k] for k in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','InspectAccess','InspectSecret')};rc.update(Enabled=True,SyntheticOnly=True,Token=f['Token'],ManifestSHA256=s['ManifestSHA256'],ValidUntil=RECOVERY_END);r.write(folder/'recovery.json',rc)
                    profile_windows['recoveries'].append({'group':group,'stage':stage,'config_sha256':r.sha(folder/'recovery.json'),'ValidUntil':rc['ValidUntil']})
                ident=r.docker('create','--pull','never','--name',project+'-'+GENERATION+'-'+stage,'--label','lifecycle307.remaining='+EXECUTION,'--network','container:'+gw,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/operator,target=/run/operator,readonly','--mount',f'type=bind,source={output},target=/run/output','--mount','type=volume,source='+budget+',target=/run/budget','--entrypoint','/run/operator',r.PG,'-stage',stage).decode().strip();owned.append(ident)
                g['Helpers'][stage]=ident;g['ConfigSHA256'][stage]=r.sha(folder/'operator.json');all_helpers.append((group,stage,folder,ident))
            root_owners={}
            for action in ('check-disabled','enable-read','enable-send','disable-send','disable-all'):
                folder=private/('root-'+action);folder.mkdir();policies=folder/'policies';policies.mkdir();output=outputs/('root-'+action);output.mkdir()
                (folder/'iam.json').write_bytes((public/'iam.json').read_bytes())
                r.write(folder/'root.json',{'Group':group,'Endpoint':'127.0.0.1:9000','RootAccess':config['RootAccess'],'RootSecret':config['RootSecret']})
                for name in native['policy_sha256']:(policies/(name+'.json')).write_bytes((verify_out/('native-policy-'+name+'.json')).read_bytes())
                ident=r.docker('create','--pull','never','--name',project+'-'+GENERATION+'-root-'+action,'--label','lifecycle307.remaining='+EXECUTION,'--network','container:'+mi,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/owner,target=/run/owner,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/owner',r.PG,'-action',action).decode().strip();owned.append(ident)
                root_owners[action]={'container':ident,'root_config_sha256':r.sha(folder/'root.json'),'scope':'trusted bootstrap only; never a worker mount'}
            candidate['Groups'].append(g)
            bindings.append({'group':group,'project':project,'workspace':s['Workspace'],'provider':s['Provider'],'incarnation':s['Incarnation'],'manifest_sha256':s['ManifestSHA256'],'plans':s['Plans'],'copy_protection':native['copies'],'checkpoint':checkpoint,'source_core_sha256':r.sha(public/'core-readonly.json'),'native_readonly_sha256':r.sha(verify_out/'native-verification.json'),'recovery_redis':rid,'budget_volume':budget,'previous_policy_heads':selected_heads,'root_owners':root_owners})
        r.write(PUBLIC/'profile-windows.json',profile_windows);candidate['ProfileWindowsSHA256']=r.sha(PUBLIC/'profile-windows.json')
        r.write(PUBLIC/'candidate.json',candidate);r.write(PUBLIC/'bindings.json',bindings)
        r.write(PUBLIC/'cross-field-windows.json',window.validate_packet(PUBLIC,PRIVATE))
        candidate_hash=r.sha(PUBLIC/'candidate.json')
        for b in bindings:
            group=b['group'];private=PRIVATE/group
            for action,owner in b['root_owners'].items():
                folder=private/('root-'+action);(folder/'candidate.json').write_bytes((PUBLIC/'candidate.json').read_bytes())
            native=next(g['Native'] for g in candidate['Groups'] if g['Group']==group);r.docker('start',native);time.sleep(3)
            ident=b['root_owners']['check-disabled']['container'];r.docker('start',ident);r.require(r.docker('wait',ident).strip()==b'0','Frozen bootstrap IAM check failed');r.docker('stop','--time','20',native)
            result=private/'outputs/root-check-disabled/result.json';(PUBLIC/group/'bootstrap-iam-check.json').write_bytes(result.read_bytes())
        lines=[f'CONFIRM_DELETE_EXACT_SEVEN；execution {EXECUTION}；{PROFILE}。',f'确认 candidate SHA256 {candidate_hash}；source {source}；source-freeze {candidate["SourceFreezeSHA256"]}；',f'Linux reader {candidate["ReaderBinarySHA256"]}；Linux sender/wire/recovery {candidate["SenderBinarySHA256"]}；target digest {target_digest}；',f'完整环境/原生保护/Core 绑定 bindings SHA256 {r.sha(PUBLIC/"bindings.json")}。',f'固定窗口 {START} 至 {END}（新加坡 {WINDOW_LABEL}），过期不能平移、延长、重开或重试。']
        lines.append(f'窗口语义投影 SHA256 {candidate["ProfileWindowsSHA256"]}；CandidateUseUntil {END}；恢复截止 {RECOVERY_END}。')
        lines.append(f'完整核验入口 SHA256 {r.sha(ROOT/"examples/lifecycle-d1/verify-remaining-native.py")}；宿主离线语义 binary {r.sha(PRIVATE/"readonly-check.exe")}；verification-build SHA256 {r.sha(PUBLIC/"verification-build.json")}。')
        for g,b in zip(candidate['Groups'],bindings):
            lines += [f'环境 {b["project"]}；workspace {b["workspace"]}；provider {b["provider"]}；incarnation {b["incarnation"]}；manifest {b["manifest_sha256"]}。',f'Core {g["Core"]}；native {g["Native"]}；gateway {g["Gateway"]}；network {g["Network"]}。',f'Core dump {g["CoreDumpSHA256"]}；globals {g["GlobalsSHA256"]}；native archive {g["NativeArchiveSHA256"]}；restore Core {g["CheckpointCore"]}；restore native {g["CheckpointNative"]}。',f'Core readonly {b["source_core_sha256"]}；native readonly {b["native_readonly_sha256"]}；IAM {g["IAMSHA256"]}；active gateway {g["GatewaySHA256"]}；sealed gateway {g["SealedGatewaySHA256"]}。']
            for cp in g['Copies']:
                stage=KEYS.get(cp['Key']);proof=next(o for o in b['copy_protection'] if o['Copy']['ID']==cp['ID'])
                plan=next((p for p in b['plans'] if p['case']==('M1' if stage in ('M1A','M1B') else stage)),None) if stage else None
                lines.append(f'{"仅删除 "+stage if stage else "必须保留"} {cp["Namespace"]}/{cp["Key"]}@{cp["ProviderVersion"]}；copy {cp["ID"]}；DV {cp["DatasetVersionID"]}；SHA256 {cp["ContentSHA256"]}；bytes {cp["ByteSize"]}；hold={proof["LegalHold"]}；retention={proof["Retention"]}；retain-until={proof["RetainUntil"]}'+(f'；plan {plan["id"]}；hash {plan["scope_hash"]}。' if plan else '。'))
            for stage,ident in g['CaseHelpers'].items():lines.append(f'case owner {stage}={ident}；profile SHA256 {g["CaseConfigSHA256"][stage]}。')
            for stage,ident in g['Helpers'].items():lines.append(f'operator owner {stage}={ident}；config SHA256 {g["ConfigSHA256"][stage]}。')
            for dv,head in g['PolicyHeads'].items():lines.append(f'未来续期 expected previous policy：DV {dv} -> {head}；漂移即拒绝。')
        for b in bindings:
            for action,owner in b['root_owners'].items():lines.append(f'bootstrap {b["group"]}/{action} owner {owner["container"]}；root-only profile SHA256 {owner["root_config_sha256"]}。')
        for row in profile_windows['recoveries']:
            lines.append(f'只读恢复 profile {row["group"]}/{row["stage"]} SHA256 {row["config_sha256"]}；独立截止 {row["ValidUntil"]}。')
        lines.append(f'受限 bootstrap Linux binary {candidate["OwnerBinarySHA256"]}；仅 MinIO admin 精确用户状态/策略读取，禁止S3 DELETE/COPY/hold API，root不挂worker。')
        lines += ['同意仅在全部执行前条件通过后，七项各最多一次 exact DELETE、全局最多七次；UNKNOWN 不重发。', '窗口开始后、任何业务 I/O 前，以新 execution 和冻结 expected heads 执行显式 Renew，UseUntil/RestoreUntil≤min(当前时刻+1小时,固定窗口结束)；保留旧policy及所有历史期限。', 'P1/P2/T1/T2 先走真实业务与owner闭合；各项必须逐份 send → 关闭发送 → 既有Outbox/Asynq只读recovery → Core精确absence回执，再允许下一份；204或UNKNOWN不代替absence，禁止连续批发后再批量恢复。H0/H1均0发送且H1只在可信owner终止、identity关闭及gateway封存的签名证明后记NOT_SENT；H2只允许A一次并制造真实204后的响应丢失，late hold不能撤销在途删除，B不得dispatch；M1先A exact absence，再B CONFIRMED UNKNOWN与C-only保护阻断mixed检查点，之后才允许B只读lookup，C永不删除。', '保留全部十一份survivor、其他环境、所有备份及失败证据；cleanup=0、hold-release=0、bypass=0；#323保持Draft、不合并；#307保持OPEN。', '旧授权全部不复用。本确认仅绑定上述候选、execution、精确版本和绝对窗口；缺少新确认或任何绑定/保护/前置漂移即停止。']
        text='\n'.join(lines)+'\n';(PUBLIC/'confirmation-template.txt').write_text(text,encoding='utf-8',newline='\n')
        for b in bindings:
            for action in b['root_owners']:
                folder=PRIVATE/b['group']/('root-'+action)
                (folder/'confirmation-template.txt').write_bytes((PUBLIC/'confirmation-template.txt').read_bytes())
                r.write(folder/'permission.json',{'Enabled':False,'Execution':EXECUTION,'CandidateSHA256':candidate_hash,'ConfirmationSHA256':r.sha(PUBLIC/'confirmation-template.txt'),'NotBefore':START,'ExpiresAt':END,'ConfirmedAt':'0001-01-01T00:00:00Z','MaxPerTarget':1,'MaxTotal':7})
        # All actual owner commands are exercised with sealed permissions while
        # their source Core/native containers are physically stopped.
        denial=[]
        for group,stage,folder,ident in all_helpers:
            (folder/'candidate.json').write_bytes((PUBLIC/'candidate.json').read_bytes());(folder/'confirmation-template.txt').write_bytes((PUBLIC/'confirmation-template.txt').read_bytes())
            r.write(folder/'permission.json',{'Enabled':False,'Profile':PROFILE,'Execution':EXECUTION,'CandidateSHA256':candidate_hash,'ConfirmationSHA256':r.sha(PUBLIC/'confirmation-template.txt'),'BinarySHA256':candidate['SenderBinarySHA256'],'NotBefore':START,'ExpiresAt':END,'ConfirmedAt':'0001-01-01T00:00:00Z','MaxPerTarget':1,'MaxTotal':7})
        for gw in source_gateways:r.docker('start',gw)
        case_denial=[]
        for g in candidate['Groups']:
            for stage,ident in g['CaseHelpers'].items():
                r.docker('start',ident);code=int(r.docker('wait',ident));logs=docker_logs(ident)
                r.require(code==1 and b'DELETE=0' in logs,'Future case owner did not deny before its frozen window')
                case_denial.append({'group':g['Group'],'stage':stage,'container':ident,'exit_code':code,'logs':logs.decode(),'source_core_native_stopped':True,'native_delete':0})
        r.write(PUBLIC/'sealed-case-regression.json',case_denial)
        for group,stage,folder,ident in all_helpers:
            r.docker('start',ident);code=int(r.docker('wait',ident));logs=docker_logs(ident)
            r.require(code==1 and b'authority denied' in logs,'Sealed real owner did not refuse before I/O')
            denial.append({'group':group,'stage':stage,'container':ident,'exit_code':code,'logs':logs.decode(),'source_core_native_stopped':True,'denial_reason':'missing fresh signed readonly topology' if stage.startswith('recover-') else 'send permission false before config/DB/provider','native_delete':0})
        r.write(PUBLIC/'sealed-owner-regression.json',denial)
        offline=PRIVATE/'offline';offline.mkdir();(offline/'candidate.json').write_bytes((PUBLIC/'candidate.json').read_bytes())
        ident=r.docker('create','--pull','never','--name','dpp-lifecycle307-'+GENERATION+'-offline','--label','lifecycle307.remaining='+EXECUTION,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={offline},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/operator,target=/run/operator,readonly','--entrypoint','/run/operator',r.PG,'-stage','check-candidate').decode().strip();owned.append(ident);r.docker('start',ident);r.require(r.docker('wait',ident).strip()==b'0','Frozen Go candidate validator failed');(PUBLIC/'offline-validator.txt').write_bytes(docker_logs(ident))
        r.write(PUBLIC/'status.json',{'candidate':PROFILE,'execution':EXECUTION,'candidate_sha256':candidate_hash,'confirmation_sha256':r.sha(PUBLIC/'confirmation-template.txt'),'source':source,'source_files':len(files),'versions':18,'prospective_targets':7,'survivors':11,'operator_owners':len(all_helpers),'case_owners':7,'human_confirmation':'NOT_RECEIVED','permission_enabled':False,'current_delete_budget':0,'native_delete':0,'hold_release':0,'bypass':0,'cleanup':0,'PR323':'DRAFT','Issue307':'OPEN','full_D1':'NOT_ACCEPTED','send_H2_UNKNOWN_M1_actual':'NOT_RUN','window_is_proposal_not_schedule':True})
        for b in bindings:
            for action in b['root_owners']:(PRIVATE/b['group']/('root-'+action)/'status.json').write_bytes((PUBLIC/'status.json').read_bytes())
    except Exception as error:
        failure=error
        r.write(PUBLIC/'freeze-failure.json',{'error_type':type(error).__name__,'error':str(error),'at':r.utc().isoformat(),'native_delete':0})
        raise
    finally:
        seal=close_known(owned+source_gateways)
        r.write(PUBLIC/'seal.json',seal)
        if not seal['all_closed'] and failure is None:raise RuntimeError('Preparation closure unconfirmed; all known owners attempted')
    complete_frozen_preflight()
    payloads={str(p.relative_to(ROOT)):r.sha(p) for p in PUBLIC.rglob('*') if p.is_file()}
    references={}
    for pattern in ('remaining-runner*','remaining-case*','remaining-executable*','remaining-policy-regression*'):
        for folder in PUBLIC.parent.glob(pattern):
            for p in folder.rglob('*'):
                if p.is_file():references[p.relative_to(ROOT).as_posix()]=r.sha(p)
    r.write(PUBLIC/'evidence-index.json',{'candidate':PROFILE,'files':{k.replace('\\','/'):v for k,v in payloads.items()},'references':references})
    print(json.dumps({'candidate':PROFILE,'execution':EXECUTION,'candidate_sha256':r.sha(PUBLIC/'candidate.json'),'source':source,'native_delete':0}))

if __name__=='__main__':main()
