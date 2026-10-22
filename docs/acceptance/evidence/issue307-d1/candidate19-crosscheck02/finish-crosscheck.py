"""Bind remaining displayed roots and public keys; no signing or provider I/O."""
import hashlib
import json
from pathlib import Path
import sys

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat

ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate19')
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import remaining_sequence

def load(path):return json.loads(path.read_bytes())
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def need(ok,message):
    if not ok:raise ValueError(message)
def save(path,value):
    with path.open('xb') as f:f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))

report=load(HERE/'crosscheck-report.json');c=load(BASE/'remaining-candidate19/candidate.json')
need(report['status']=='PASS_C19_V29_DISPLAY_AND_FROZEN_EVIDENCE_CROSSCHECK' and not report['remaining_inconsistencies'],'Prior crosscheck incomplete')
text=(HERE/'prior-display.md').read_text(encoding='utf-8')
anchor=load(PRIVATE/'host-anchor.json');sf=load(BASE/'remaining-candidate19/source-freeze.json')
rows={
    f'source ZIP，{sf["archive_bytes"]} bytes／{len(sf["files"])} 选定源码成员':sf['archive_sha256'],
    'containment 专用 packet Git anchor':anchor['packet_commit'],
    '私有 host-anchor 文件':sha(PRIVATE/'host-anchor.json'),
    '当前发布 HEAD':report['head']}
for label,value in rows.items():need('| '+label+' | `'+value+'` |' in text,'Additional displayed binding differs: '+label)
blocks=text.split('```text\n')
logs=blocks[2].split('```',1)[0].splitlines()
need(set(logs)=={name for name in sf['initial_build_files'] if name.endswith(('.stdout','.stderr'))} and len(logs)==8,'Eight original build log names differ')
need(all(sf['initial_build_files'][name]=='e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855' for name in logs),'Original empty build logs differ')
fields=('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')
targets=[{key:cp[key] for key in fields} for _,_,cp in remaining_sequence.targets(c)]
computed=hashlib.sha256(json.dumps(targets,ensure_ascii=False,separators=(',',':')).encode('utf-8')).hexdigest()
need(computed==c['TargetDigest'],'Six-target digest differs')
host_public=Ed25519PrivateKey.from_private_bytes((PRIVATE/'topology-key.private').read_bytes()).public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
need(host_public.hex()==c['PublicKey'] and c['WirePublicKey']!=c['PublicKey'],'Host public/private identity differs or wire key aliases host')
wire_count=0;nonwire_count=0
for group in c['Groups']:
    for stage in group['Helpers']:
        config=load(PRIVATE/group['Group']/stage/'operator.json')
        if stage=='wire':
            private=bytes.fromhex(config['WirePrivateKey'])
            need(len(private)==64 and private[32:].hex()==c['WirePublicKey'] and Ed25519PrivateKey.from_private_bytes(private[:32]).public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)==private[32:],'Wire public/private identity differs')
            wire_count+=1
        else:
            need(config['WirePrivateKey']=='','Wire private key exposed to a nonwire owner');nonwire_count+=1
need(wire_count==3 and nonwire_count==12,'Key-owner membership differs')
result={'status':'PASS_ADDITIONAL_DISPLAY_ROOTS_BUILD_LOGS_TARGET_DIGEST_AND_PUBLIC_KEYS',
    'rows':rows,'original_empty_log_members':logs,'target_digest':computed,
    'host_public':c['PublicKey'],'wire_public':c['WirePublicKey'],'wire_owners':3,'nonwire_owners_without_wire_key':12,
    'proof_signatures':0,'source_provider_io':0,'permission_enable':0,'native_delete':0}
save(HERE/'additional-bindings.json',result)
matrix=load(HERE/'exact-version-matrix.json')['versions']
counts=[]
for group in ('business-races','late-hold','mixed'):
    group_rows=[row for row in matrix if row['group']==group]
    counts.append((group,len(group_rows),sum(row['target'] for row in group_rows),sum(not row['target'] for row in group_rows)))
table='| 环境组 | 现存版本 | 目标 | 保留 |\n| --- | ---: | ---: | ---: |\n'+''.join(f'| {g} | {n} | {t} | {s} |\n' for g,n,t,s in counts)
summary=f'''# #307 D1 candidate19/v29 逐项核对

核对通过，remaining_inconsistencies=[]。本轮核对的是本聊天实际展示消息及当前冻结字节，不是仅拿两个文件互比。127行模板逐字节一致，SHA `{report['template_sha256']}`；36宿主成员、12项显示附录根绑定、893审批引用、7索引4539成员、2143源码raw/LF/Git/ZIP及12原构建成员匹配。P1精确历史身份及原NoSuchVersion404保留，不计入现存版本、六目标或新预算。

execution `{report['execution']}`；2026-10-22新加坡14:00≤now<14:40（UTC06:00≤now<06:40）；只读恢复截止新加坡16:40（UTC08:40）。六实际case和六recovery跨字段核验通过，CandidateUseUntil严格等于固定END。

{table}

逐份namespace/key/version、copy、DV、bytes/SHA、plan/hash、hold/retention/retain-until均与候选、bindings和已保存的真实native回执一致。顺序P2→T1→T2→H2A→M1A→M1B，每份max1/全局max6，UNKNOWN不重发。三组Windows只读checker仅回放旧真实回执，全部PASS；不冒充当前fresh provider状态。

当前物理冷状态：820已知CID关闭/PID0/noports/norestart；30permission=false；21旧预算原字节和挂载集保留，3新独立预算为空，57备份SHA匹配。host/wire公钥与对应私有材料一致且分离，3wire持有wire私钥、其余12operator无wire私钥；本轮签名0。

实时GitHub：HEAD `{report['head']}`，PR CI run39710200629 event=pull_request、13/13 SUCCESS；未解线程0；#332 OPEN/Draft/unmerged，#307 OPEN。运行source仍`{report['runtime_source']}`，tracked工作区干净。本轮仅追加未跟踪核对回执，没有改冻结候选/source/binary/config/HEAD，没有创建授权或调度。

执行仍需新精确人类授权；若需定时续跑，另按人类指令建立。窗口内还必须重新完成新鲜Core/native/IAM/17版本/保护状态/拓扑预算核验，完整入口finally关闭后先实际授权Host.check，再有限启用；sender/wire/recovery各自需≤1分钟fresh host proof。当前Host.check=NOT_RUN_REQUIRES_NEW_HUMAN_AUTHORIZATION；窗口内fresh核验=NOT_RUN；不能把本次静态和冷态PASS当全部执行前置已通过。

原失败、已用预算、旧pins和备份保留；cleanup=0、hold-release=0、bypass=0。不得复用旧授权、重发UNKNOWN、扩窗、补发或退款；失败/过窗逐组固定seal。crosscheck01首轮失败是核对脚本误将18历史copy当17现存库存，原脚本/失败保留；修正只在核对工具内，未改变冻结运行门禁或候选。

本轮source_start=0、source_provider_io=0、permission_enable=0、proof_signatures=0、DELETE=0；六目标发送仍NOT_RUN，完整D1 NOT_ACCEPTED。
'''
with (ROOT/'docs/acceptance/issue307-d1-v29-crosscheck01.md').open('x',encoding='utf-8',newline='\n') as f:f.write(summary)
paths=[path for path in HERE.rglob('*') if path.is_file()]
paths.append(ROOT/'docs/acceptance/issue307-d1-v29-crosscheck01.md')
save(HERE/'evidence-index.json',{'status':'C19_V29_CROSSCHECK_EVIDENCE_INDEX','count':len(paths),
    'files':{path.relative_to(ROOT).as_posix():{'sha256':sha(path),'bytes':path.stat().st_size} for path in sorted(paths)},
    'root':'.','authorization':False,'native_delete':0,'immutable_frozen_approval_manifest_unchanged':True})
print(json.dumps({'status':result['status'],'target_digest':computed,'evidence_members':len(paths),'native_delete':0}))
