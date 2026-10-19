"""Freeze candidate13 only after actual bounded non-delete preparation.

No IAM enable, deletion, authorization or scheduling function exists here.
The only container run is the already allocated network-none file validator.
"""
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess

spec=importlib.util.spec_from_file_location('six_freeze_runtime',Path(__file__).with_name('six_candidate_runtime.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
p=r.p
need,load,sha,save=r.need,r.load,r.sha,r.save
PACKET=r.ROOT/'docs/acceptance/evidence/issue307-d1'/('remaining-'+p.GENERATION)
COPY_FIELDS=('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')


def digest(raw):return hashlib.sha256(raw).hexdigest()
def compact(value):return json.dumps(value,ensure_ascii=False,separators=(',',':')).encode()
def cp_order(cp):return {key:cp[key] for key in COPY_FIELDS}


def source_freeze(a):
    names=subprocess.check_output(['git','ls-files','apps/platform','examples/lifecycle-d1','migrations'],cwd=r.ROOT).decode('utf-8','strict').splitlines()
    files={}
    for name in names:
        path=r.ROOT/name
        if path.suffix not in {'.go','.py','.ps1','.sql','.json','.mod','.sum','.yml','.yaml','.Caddyfile'}:continue
        raw=path.read_bytes();git=subprocess.check_output(['git','show',a['source']+':'+name],cwd=r.ROOT)
        need(raw.replace(b'\r\n',b'\n')==git.replace(b'\r\n',b'\n'),'build source and current source differ')
        files[name]={'raw_sha256':digest(raw),'lf_sha256':digest(raw.replace(b'\r\n',b'\n'))}
    archive=r.CONTROL/'approved-runtime-source.zip'
    need(not archive.exists(),'runtime source archive exists')
    p.call(['git','archive','--format=zip',a['source'],'apps/platform','examples/lifecycle-d1','migrations'],output=archive)
    save(PACKET/'source-freeze.json',{'source':a['source'],'files':files,'archive_sha256':sha(archive),'archive_bytes':archive.stat().st_size,
                                    'build_recipe':'go build -trimpath -buildvcs=false; linux/amd64 CGO_ENABLED=0; Windows file-only checker',
                                    'embedded_vcs_revision':'NOT_CLAIMED','actual_build_source':a['source'],
                                    'initial_allocation_build_source':a['initial_build_source'],'source_checkpoint_sha256':a['source_checkpoint_sha256'],
                                    'identical_rebuild_receipt_sha256':a['rebuild_receipt_sha256'],'active_original_binary_bytes_preserved':True})


def main():
    a=r.state();r.preparation_gate(a)
    terminal=load(r.PUBLIC/'non-delete-terminal.json')
    need(terminal['status']=='PASS_NONDELETE_SOURCE_RESTORE_PREPARATION' and terminal['groups_completed']==3 and terminal['native_delete']==0,'actual source/restore preparation incomplete')
    need(sha(r.PUBLIC/'non-delete-evidence-index.json')==terminal['evidence_index_sha256'],'original preparation index drift')
    for name,expected_sha in load(r.PUBLIC/'non-delete-evidence-index.json')['files'].items():
        path=(r.PUBLIC/name).resolve();need(path.is_relative_to(r.PUBLIC.resolve()) and sha(path)==expected_sha,'original raw preparation member drift')
    need(not PACKET.exists(),'one-shot frozen packet exists; never overwrite')
    PACKET.mkdir()
    source_freeze(a)
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding,PrivateFormat,PublicFormat,NoEncryption
    host,wire=Ed25519PrivateKey.generate(),Ed25519PrivateKey.generate()
    host_public=host.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()
    wire_public=wire.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
    r.write_raw(r.CONTROL/'topology-key.private',host.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
    wire_private=(wire.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption())+wire_public).hex()
    old=load(p.p1.PACKET/'candidate.json');targets=[]
    for name,stage,key in p.sequence.ORDER:
        g=next(x for x in old['Groups'] if x['Group']==name);targets.append(cp_order(next(x for x in g['Copies'] if x['Key']==key)))
    candidate={'Profile':p.PROFILE,'Execution':p.EXECUTION,'Source':a['source'],'SourceFreezeSHA256':sha(PACKET/'source-freeze.json'),
               'ReaderBinarySHA256':a['binaries']['reader'],'SenderBinarySHA256':a['binaries']['operator'],'OwnerBinarySHA256':a['binaries']['owner'],
               'TargetDigest':digest(compact(targets)),'PublicKey':host_public,'WirePublicKey':wire_public.hex(),'NotBefore':p.W['NotBefore'],'NotAfter':p.W['NotAfter'],'Groups':[]}
    bindings=[];windows={'cases':[],'recoveries':[]}
    for allocation in a['groups']:
        group=allocation['group'];folder=Path(allocation['private']);public=PACKET/group;public.mkdir()
        old_group=next(x for x in old['Groups'] if x['Group']==group);scope=allocation['scope'];project='dpp-lifecycle307-'+scope['Run'][:8]
        checkpoint=load(r.PUBLIC/group/'checkpoint.json')
        receipt=load(r.PUBLIC/group/'maintenance-source/files/six-native-verification.json')['verification']
        # Keep current actual seventeen and historical eighteen as separate
        # fields; Core still registers P1, which is never a current target.
        iam={'disabled_identities':receipt['disabled_identities'],'policy_sha256':receipt['policy_sha256'],
             'policy_file_sha256':{name:sha(r.PUBLIC/group/'maintenance-source/files'/('native-policy-'+name+'.json')) for name in receipt['policy_sha256']}}
        save(public/'iam.json',iam)
        shutil.copytree(r.PUBLIC/group/'expectation',public/'expectation')
        shutil.copytree(r.PUBLIC/group/'maintenance-source',public/'native-readonly')
        r.write_raw(public/'core-readonly.json',(r.PUBLIC/group/'source-Core-after.json').read_bytes())
        save(public/'checkpoint.json',checkpoint)
        access=p.identity(scope,'inspect')
        active=("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n @metadata {\n method GET HEAD\n expression {http.request.header.Authorization}.contains('Credential="+access+"/')\n }\n reverse_proxy @metadata minio:9000\n respond 403\n }\n}\n").encode()
        sealed=(folder/'gateway.Caddyfile').read_bytes()
        for name,raw in [('gateway-active.Caddyfile',active),('gateway-sealed.Caddyfile',sealed)]:r.write_raw(folder/name,raw);r.write_raw(public/name,raw)
        config=load(p.p1.PRIVATE/project/'config.json');secrets=load(folder/'new-secrets.private.json')
        heads=load(folder/('case-'+next(iter(allocation['cases'])))/'remaining.json')['ExpectedPolicyHeads']
        g={'Group':group,'Actor':config['ActorID'],**allocation['source'],'Gateway':allocation['gateway'],'CheckpointCore':allocation['restore_core'],'CheckpointNative':allocation['restore_native'],
           'CoreDumpSHA256':checkpoint['backups']['core.dump'],'GlobalsSHA256':checkpoint['backups']['globals.sql'],'NativeArchiveSHA256':checkpoint['backups']['native.tar'],
           'IAMSHA256':sha(public/'iam.json'),'GatewaySHA256':digest(active),'SealedGatewaySHA256':digest(sealed),'Copies':old_group['Copies'],'Helpers':allocation['helpers'],
           'ConfigSHA256':{},'CaseHelpers':allocation['cases'],'CaseConfigSHA256':{},'PolicyHeads':heads}
        for stage in g['CaseHelpers']:
            path=folder/('case-'+stage)/'remaining.json';cfg=load(path)
            if stage=='close-H1':
                cfg.update(ClosurePublicKey=host_public,ClosureCandidateSHA256=digest(compact({'execution':p.EXECUTION,'source':a['source'],'reader':candidate['ReaderBinarySHA256'],'target':candidate['TargetDigest'],'owner':g['CaseHelpers']['late']})),
                           ClosureOwners={g['CaseHelpers']['late']:candidate['ReaderBinarySHA256']})
                r.replace(path,cfg)
            g['CaseConfigSHA256'][stage]=sha(path);windows['cases'].append(p.window.project_case(cfg,group,stage,sha(path)))
        token=load(folder/('case-'+next(iter(g['CaseHelpers'])))/'remaining.json')['Base']['Token']
        for stage in g['Helpers']:
            target=folder/stage
            operator=dict(Group=group,PostgresDSN=config['PostgresDSN'],InspectAccess=access,InspectSecret=secrets[access],DeleteAccess=p.identity(scope,'delete'),
                          DeleteSecret='' if stage=='wire' or stage.startswith('recover-') else secrets[p.identity(scope,'delete')],Token=token,
                          WirePrivateKey=wire_private if stage=='wire' else '',RedisAddr=project+'-'+p.GENERATION+'-recovery-redis:6379')
            save(target/'operator.json',operator);g['ConfigSHA256'][stage]=sha(target/'operator.json')
            if stage.startswith('recover-'):
                recovery={key:config[key] for key in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint')}
                recovery.update(InspectAccess=access,InspectSecret=secrets[access],Enabled=True,SyntheticOnly=True,Token=token,ManifestSHA256=scope['ManifestSHA256'],ValidUntil=p.W['RecoveryEnd'])
                save(target/'recovery.json',recovery);windows['recoveries'].append({'group':group,'stage':stage,'config_sha256':sha(target/'recovery.json'),'ValidUntil':p.W['RecoveryEnd']})
        owners={}
        for action,cid in allocation['root_owners'].items():
            target=folder/('root-'+action);policies=target/'policies';policies.mkdir()
            save(target/'root.json',{'Group':group,'Endpoint':'127.0.0.1:9000','RootAccess':config['RootAccess'],'RootSecret':config['RootSecret']})
            r.write_raw(target/'iam.json',(public/'iam.json').read_bytes())
            for name in iam['policy_file_sha256']:r.write_raw(policies/(name+'.json'),(r.PUBLIC/group/'maintenance-source/files'/('native-policy-'+name+'.json')).read_bytes())
            owners[action]={'container':cid,'root_config_sha256':sha(target/'root.json')}
        candidate['Groups'].append(g)
        bindings.append({'group':group,'project':project,'workspace':scope['Workspace'],'provider':scope['Provider'],'incarnation':scope['Incarnation'],'manifest_sha256':scope['ManifestSHA256'],'plans':scope['Plans'],
                         'copy_protection':receipt['copies'],'checkpoint':checkpoint,'source_core_sha256':sha(public/'core-readonly.json'),'native_readonly_sha256':sha(r.PUBLIC/group/'maintenance-source/files/six-native-verification.json'),
                         'recovery_redis':allocation['recovery_redis'],'budget_volume':allocation['volumes']['budget'],'budget_initializer':allocation['budget_initializer'],'budget_initializer_script_sha256':allocation['budget_initializer_script_sha256'],
                         'root_owners':owners,'allocation_sha256':sha(r.PUBLIC/'allocation.json'),'owner_descriptors_sha256':a['owner_descriptors_sha256'],
                         'map_sha256':a['map_sha256'],'preparation_terminal_sha256':sha(r.PUBLIC/'non-delete-terminal.json'),'previous_policy_heads':heads})
        bindings[-1].update(initial_build_source=a['initial_build_source'],source_checkpoint_sha256=a['source_checkpoint_sha256'],rebuild_receipt_sha256=a['rebuild_receipt_sha256'])
    save(PACKET/'profile-windows.json',windows);candidate['ProfileWindowsSHA256']=sha(PACKET/'profile-windows.json')
    save(PACKET/'candidate.json',candidate);save(PACKET/'bindings.json',bindings)
    confirmation(candidate,bindings,targets)
    candidate_sha=sha(PACKET/'candidate.json');template_sha=sha(PACKET/'confirmation-template.txt')
    common=dict(Enabled=False,Execution=p.EXECUTION,CandidateSHA256=candidate_sha,ConfirmationSHA256=template_sha,NotBefore=p.W['NotBefore'],ExpiresAt=p.W['NotAfter'],ConfirmedAt=r.ZERO,MaxPerTarget=1,MaxTotal=6)
    for g,b in zip(candidate['Groups'],bindings):
        for stage in g['Helpers']:
            folder=r.CONTROL/g['Group']/stage;r.replace(folder/'permission.json',dict(common,Profile=p.PROFILE,BinarySHA256=candidate['SenderBinarySHA256']))
            r.write_raw(folder/'candidate.json',(PACKET/'candidate.json').read_bytes());r.write_raw(folder/'confirmation-template.txt',(PACKET/'confirmation-template.txt').read_bytes())
        for action in b['root_owners']:
            folder=r.CONTROL/g['Group']/('root-'+action);r.replace(folder/'permission.json',common)
            r.write_raw(folder/'candidate.json',(PACKET/'candidate.json').read_bytes());r.write_raw(folder/'confirmation-template.txt',(PACKET/'confirmation-template.txt').read_bytes())
            save(folder/'status.json',{'candidate_sha256':candidate_sha,'confirmation_sha256':template_sha})
    save(PACKET/'cross-field-windows.json',p.window.validate_packet(PACKET,r.CONTROL))
    r.write_raw(r.CONTROL/'offline/candidate.json',(PACKET/'candidate.json').read_bytes())
    r.run_owner(a['offline_validator'],PACKET/'offline-validator')
    save(PACKET/'status.json',{'candidate':p.PROFILE,'execution':p.EXECUTION,'candidate_sha256':candidate_sha,'bindings_sha256':sha(PACKET/'bindings.json'),'confirmation_sha256':template_sha,
                              'source':a['source'],'historical_copies':18,'present_versions':17,'targets':6,'survivors':11,'human_confirmation':'NOT_RECEIVED','permission_enabled':False,'native_delete':0,
                              'full_D1':'NOT_ACCEPTED','Issue307':'OPEN','PR332':'DRAFT','old_authorization_reused':False,'schedule':'NOT_CREATED'})
    print(json.dumps({'status':'SIX_PACKET_FROZEN_NOT_AUTHORIZED','candidate_sha256':candidate_sha,'execution':p.EXECUTION}))


def confirmation(c,bindings,targets):
    sgt=dt.timezone(dt.timedelta(hours=8));start=p.window.instant(c['NotBefore']).astimezone(sgt);end=p.window.instant(c['NotAfter']).astimezone(sgt)
    lines=[f'CONFIRM_DELETE_EXACT_SIX；execution {p.EXECUTION}；{p.PROFILE}。',
           f'固定窗口 {start:%Y-%m-%d %H:%M}–{end:%H:%M} 新加坡时间；UTC {c["NotBefore"]}≤now<{c["NotAfter"]}；只读恢复截止 {p.W["RecoveryEnd"]}。',
           f'candidate SHA256 {sha(PACKET/"candidate.json")}；bindings {sha(PACKET/"bindings.json")}；source {c["Source"]}；source-freeze {c["SourceFreezeSHA256"]}。',
           f'初始allocation构建源 {bindings[0]["initial_build_source"]}；追加source checkpoint {bindings[0]["source_checkpoint_sha256"]}；新source四份完全相同字节重编译证明 {bindings[0]["rebuild_receipt_sha256"]}；原allocation/旧binary/冷态失败原字节保留。',
           f'Linux reader {c["ReaderBinarySHA256"]}；sender/wire/recovery {c["SenderBinarySHA256"]}；root owner {c["OwnerBinarySHA256"]}；Windows file-only checker {sha(r.CONTROL/"readonly-check.exe")}。',
           f'target digest {c["TargetDigest"]}；host public key {c["PublicKey"]}；wire public key {c["WirePublicKey"]}；profile-windows {c["ProfileWindowsSHA256"]}；CandidateUseUntil={c["NotAfter"]}。',
           '18历史copy、17现存版本、6目标、11保留版本严格分开。旧P1已确认缺失，绝不纳入目标或新预算。']
    for g,b in zip(c['Groups'],bindings):
        lines.extend([f'环境 {b["project"]}；workspace {b["workspace"]}；provider {b["provider"]}；incarnation {b["incarnation"]}；manifest {b["manifest_sha256"]}。',
                      f'Core {g["Core"]}；native {g["Native"]}；gateway {g["Gateway"]}；network {g["Network"]}；restore Core {g["CheckpointCore"]}；restore native {g["CheckpointNative"]}。',
                      f'Core dump {g["CoreDumpSHA256"]}；globals {g["GlobalsSHA256"]}；native archive {g["NativeArchiveSHA256"]}；source Core {b["source_core_sha256"]}；native receipt {b["native_readonly_sha256"]}。',
                      f'IAM {g["IAMSHA256"]}；active gateway {g["GatewaySHA256"]}；sealed gateway {g["SealedGatewaySHA256"]}；allocation {b["allocation_sha256"]}；owner descriptors {b["owner_descriptors_sha256"]}；environment map {b["map_sha256"]}。',
                      f'新预算 {b["budget_volume"]}；initializer CID {b["budget_initializer"]}；initializer script {b["budget_initializer_script_sha256"]}；Redis {b["recovery_redis"]}；preparation terminal {b["preparation_terminal_sha256"]}。'])
        for stage,cid in g['Helpers'].items():lines.append(f'operator {g["Group"]}/{stage} CID {cid}；config {g["ConfigSHA256"][stage]}。')
        for stage,cid in g['CaseHelpers'].items():lines.append(f'case {g["Group"]}/{stage} CID {cid}；config {g["CaseConfigSHA256"][stage]}。')
        for action,owner in b['root_owners'].items():lines.append(f'root {g["Group"]}/{action} CID {owner["container"]}；config {owner["root_config_sha256"]}。')
        iam=load(PACKET/g['Group']/'iam.json')
        for user,name in iam['disabled_identities'].items():lines.append(f'IAM identity {user}=disabled；sole policy {name}；无groups；policy file SHA256 {iam["policy_file_sha256"][name]}。')
        for cp in g['Copies']:
            if cp['Key']=='fixture/p1.csv':continue
            target=any(cp['ID']==t['ID'] for t in targets)
            case={'fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2','fixture/h0.csv':'H0','fixture/h1-a.csv':'H1','fixture/h1-b.csv':'H1','fixture/h2-a.csv':'H2','fixture/h2-b.csv':'H2','fixture/m1-a.csv':'M1','fixture/m1-b.csv':'M1','fixture/m1-c.csv':'M1'}.get(cp['Key'])
            plan=next((x for x in b['plans'] if x['case']==case),None)
            need(not target or plan is not None,'target plan identity missing')
            protection=next(x for x in b['copy_protection'] if x['Copy']['ID']==cp['ID'])
            lines.append(('删除目标' if target else '保留版本')+f' {b["project"]} {cp["Namespace"]}/{cp["Key"]}@{cp["ProviderVersion"]}；copy {cp["ID"]}；DV {cp["DatasetVersionID"]}；SHA256 {cp["ContentSHA256"]}；bytes {cp["ByteSize"]}；hold={protection["LegalHold"]}；retention={protection["Retention"]}；retain-until={protection["RetainUntil"]}；'+(f'plan {plan["id"]}；hash {plan["scope_hash"]}。' if plan else '非发送目标。'))
    lines.extend(['严格顺序P2→T1→T2→H2A→M1A→M1B；每份至多一次、全局至多六次，UNKNOWN不重发；每份send后关闭发送，经既有选定Outbox→Asynq只读恢复并提交absence，再进入下一份。三Core保持可读以核全局顺序。',
                  'H0/H1零发送；可信owner终止、profile撤销、enabler关闭、fresh disabled-IAM及sealed gateway后H1才NOT_SENT。H2仅A真实204后witness/late hold A+B/revoke/响应丢失，B永不dispatch。M1先A absence，B UNKNOWN+C-only保护阻断checkpoint原字节复制并核SHA后才B lookup，C永不删除。',
                  '全部新鲜前置及精确当前HEAD PR CI/评审通过后才可启用发送；每个sender/wire/recovery前≤1分钟新鲜实际host topology；wire context≤3分钟，每目标刷新；SDK无重试，持久预算先于Core claim。',
                  '11份survivor、其他全部环境、全部旧/新备份、旧pins、旧预算、失败/UNKNOWN/真实attempt/成本事实保留。cleanup=0；hold-release=0；bypass=0。不得退款、补发、重发UNKNOWN、修改旧事实或复用旧授权。',
                  '失败或过窗立即停止新增发送并逐组封存；不延长、平移、重开窗口。只读恢复期限不得重新启用身份或延长发送；关闭IAM后无法lookup即保留UNKNOWN/NOT_RUN。#332 Draft、不合并；#307 OPEN；完整D1不提前验收。'])
    r.write_raw(PACKET/'confirmation-template.txt',('\n'.join(lines)+'\n').encode('utf-8'))


if __name__=='__main__':main()
