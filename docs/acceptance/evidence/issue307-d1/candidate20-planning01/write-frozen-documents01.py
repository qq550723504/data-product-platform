"""Render v30 preparation documents from exact frozen bytes; no runtime I/O."""
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[5]
DOCS=ROOT/'docs/acceptance'
BASE=DOCS/'evidence/issue307-d1'
HERE=Path(__file__).parent
PACKET=BASE/'remaining-candidate20'
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate20')
load=lambda p:json.loads(p.read_bytes())
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def need(ok,message):
    if not ok:raise ValueError(message)


def write(path,text):
    with path.open('xb') as f:f.write(text.encode('utf-8'))


def main():
    c=load(PACKET/'candidate.json');s=load(PACKET/'status.json');sf=load(PACKET/'source-freeze.json')
    complete=load(BASE/'remaining-candidate20-readonly01/complete.json')
    prep=load(BASE/'remaining-candidate20-preparation01/non-delete-terminal.json')
    anchor=load(HERE/'host-anchor-readback01.json')['anchor']
    template=(PACKET/'confirmation-template.txt').read_bytes()
    windows=load(PACKET/'profile-windows.json')
    recovery_ends={x['ValidUntil'] for x in windows['recoveries']}
    need(len(windows['recoveries'])==6 and len(recovery_ends)==1,'recovery deadline membership differs')
    recovery_end=next(iter(recovery_ends))
    need(c['Profile']=='candidate20/v30' and c['Execution']=='5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6','current scope differs')
    need(complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY' and complete['present_versions']==17 and complete['native_delete']==0,'complete actual read missing')
    need(prep['status']=='PASS_NONDELETE_SOURCE_RESTORE_PREPARATION' and prep['groups_completed']==3,'native preparation missing')
    need(template.count('删除目标'.encode())==6 and template.count('保留版本 '.encode())==11,'exact target/survivor membership differs')
    host_files=[ROOT/'examples/lifecycle-d1'/name for name in (
        'prepare-six-candidate.py','six_candidate_runtime.py','freeze-six-candidate.py',
        'verify-six-native.py','window_six_support.py','attest-remaining-executable.py',
        'remaining_window.py','remaining_six_window.py','remaining_sequence.py',
        'evidence_indexes.py','docker_host_paths.py','retained_v26.py','retained_v27.py','retained_v28.py','retained_v29.py','fixture_artifacts.py','fixture-runtime-artifact.json','execution_freeze.py','branch-freeze.py')]
    facts=[PACKET/name for name in ('candidate.json','bindings.json','confirmation-template.txt',
        'source-freeze.json','profile-windows.json','cross-field-windows.json',
        'preparation-evidence-audit-plan.json','source-review-binding-index.json')]
    facts += [BASE/'candidate20-artifacts-review01'/name for name in ('independent-final-review01.json','evidence-index.json')]
    facts += [HERE/name for name in ('host-anchor-readback01.json','engine-service-bindings01.json','allocation-readback01.json')]
    facts += [BASE/'remaining-candidate20-readonly01/complete.json']
    facts += [BASE/'remaining-candidate20-preparation01'/name for name in ('non-delete-terminal.json','non-delete-evidence-index.json','allocation.json','owner-descriptors.json','retained-before.json','budget-initialization.json')]
    facts += [ROOT/'docs/acceptance/evidence/issue307-d1/remaining-window-review06/composition/window_support.py']
    members=host_files+facts
    need(len(set(members))==len(members),'duplicate host binding')
    bindings={'status':'FROZEN_V29_HOST_AND_PREPARATION_BINDINGS','profile':c['Profile'],
        'execution':c['Execution'],'source':c['Source'],'packet_commit':anchor['packet_commit'],
        'candidate_sha256':s['candidate_sha256'],'bindings_sha256':s['bindings_sha256'],
        'template_sha256':s['confirmation_sha256'],'not_before':c['NotBefore'],'not_after':c['NotAfter'],
        'count':len(members),'files':{p.relative_to(ROOT).as_posix():{'sha256':sha(p),'bytes':p.stat().st_size} for p in members},
        'initial_build_files':sf['initial_build_files'],'source_zip_sha256':sf['archive_sha256'],
        'source_zip_bytes':sf['archive_bytes'],'authorization':'NOT_RECEIVED','schedule':'NOT_CREATED','authority':False}
    write(HERE/'candidate20-host-bindings01.json',json.dumps(bindings,ensure_ascii=False,indent=2)+'\n')
    table='| 冻结成员 | SHA256 |\n| --- | --- |\n'+''.join(f'| `{p.relative_to(ROOT).as_posix()}` | `{sha(p)}` |\n' for p in members)
    builds='| 原构建成员 | SHA256 |\n| --- | --- |\n'+''.join(f'| `{name}` | `{digest}` |\n' for name,digest in sf['initial_build_files'].items())
    preface=f'''# #307 D1 candidate20/v30 完整精确确认方案

当前候选制品 FROZEN_NOT_AUTHORIZED，最终分支锁尚待独立评审/最终HEAD CI后另存；未有execution-binding SHA/最终HEAD/ruleID/完整保护附录不得授权或调度。新 execution `{c['Execution']}`，拟定2026-10-22新加坡18:00–18:40（UTC10:00≤now<10:40）；只读恢复截止2026-10-22T12:40Z，即新加坡10月22日20:40。本轮只准备方案，没有新的删除授权或调度；#332 Draft、不合并，#307 OPEN。唯一工作目录为 `C:/Users/Henry/.codex/worktrees/d1-lock/data-product-platform`，私有控制目录为 `C:/Users/Henry/.codex/fixture-private/remaining-candidate20`。

完整机器文本共{len(template.splitlines())}行，UTF-8/LF/末尾LF，SHA256 `{sha(PACKET/'confirmation-template.txt')}`。下方原文与冻结模板逐字节一致。最终原生分支锁及execution-binding公开/私有同字节收据只能在最终提交之后追加，不能推进锁定HEAD；人类实际授权须同时绑定该附录。未来按引用授权时，实际人类原文与机器模板分别留证，user_retyped_template=false；旧授权不复用。

```text
'''
    suffix=f'''```

## 宿主、恢复与证据绑定

fixture运行source R `{c['Source']}`；{len(sf['runtime']['files'])}R归档源码成员固定原字节SHA；宿主P {sf['host']['source']} 的{len(sf['host']['files'])}成员独立记录raw/LF SHA，source ZIP `{sf['archive_sha256']}`、{sf['archive_bytes']} bytes。12原始构建成员如下，不能只重编译active binary冒充原构建。packet Git anchor `{anchor['packet_commit']}`，私有host-anchor SHA `{sha(PRIVATE/'host-anchor.json')}`，只供按相同SHA定位已批准关闭CID，不授予发送。

{builds}

宿主绑定索引 `{sha(HERE/'candidate20-host-bindings01.json')}`，包含下表全部实际字节；case/recovery、完整owner descriptor、mount/network/volume与预算归属见allocation和bindings。

{table}

三源当前Core业务基线在本代bindings中固定。business-races为本次v28可信混合闭合后的全关系指纹，另两组保留原完整指纹；aab0 policy checkpoint及五个head/已过期期限仍是历史事实，不Renew，不把它的历史CoreSHA当当前全关系指纹。原v28 P2 CONFIRMED DENIED/已用预算、T1/T2未发送以及之后可信混合闭合、新17版本只读结果分别保留，不回写原终态。

本代三组原生非删除准备、9新备份、独立clean/rehearsal Core/native恢复及隔离业务重跑、Go离线校验、完整原生只读入口均通过。完整入口核17版本、47已知disabled身份（46 sole-policy/no-groups及唯一固定NONE/no-groups）、9 fresh NoSuchUser；本次保存{complete['technical_requests']}技术请求start/observation对，amount=NOT_ASSESSED。这些是准备事实，不能替代将来窗口内fresh核验。

R仅支持固定schema69/66且无identity_kind，宿主P保留当前产品保护模型；同一source-freeze v2固定两制品、共享窗口/scope/policy和原构建，禁止混用。三真实源未升级，隔离schema升级提案未授权且不属于本方案。执行入口先核R/P原字节及交叉字段，再允许Docker/source。最终冻结包独立报告须PASS且remaining_blockers=[]。完整审批引用附录为 `candidate20-planning01/approval-reference-manifest01.json`，其最终静态audit plan由同一固定入口核验；不在文档中写入其自身哈希以制造引用环。未来实际授权、批准模板、完整reference manifest/envelope和后续调度事实另存，准备NOT_RECEIVED快照不改写。

执行前依次核精确发布HEAD与全部必要PR CI/评审线程、#332 Draft/#307 OPEN及全部批准字节；固定完整入口新鲜检查case/window、全部拓扑/预算、Core/native/IAM/17版本、全部备份，入口finally关闭后先Host.check核新授权及停止状态。只有固定窗口内全部前置通过才允许有限启用。每份send→disable-send→选定Outbox→既有Asynq只读recover→Core immutable absence→下一份；每sender/wire/recovery前≤1分钟fresh实际host proof，wire context≤3分钟且逐target刷新。H0/H1零send，H2真实204之后才late hold/revoke/丢响应，M1先A absence，再B UNKNOWN+C-only checkpoint原字节复制核SHA后才B lookup。

SDK无重试，持久target/wire budget先于Core claim，UNKNOWN不重发。失败或到END立即停止新增enable/wire/sender并逐组固定seal；归档失败仍继续全部disable-all/物理停止，缺fresh关闭读回即UNCONFIRMED。11份保留版本、其他环境、全部备份、旧pins、新旧预算、原失败/UNKNOWN/attempt/成本事实全部保留；cleanup=0、hold-release=0、bypass=0。不延长、平移、重开、补发、退款或释放旧pins。完整步骤见[v30手册](issue307-d1-v30-window-runbook.md)，实际状态见[v30矩阵](issue307-d1-v30-preparation.md)。PR CI、main CI、运行证据与人类授权分别核对；完整D1仍NOT_ACCEPTED。
'''
    write(DOCS/'issue307-d1-candidate20-confirmation.md',preface+template.decode()+suffix)
    runbook=f'''# #307 D1 candidate20/v30 有限窗口手册

本代execution `{c['Execution']}`，拟定2026-10-22新加坡18:00–18:40，即UTC10:00≤now<10:40；只读恢复最晚12:40Z（新加坡10月22日20:40）。当前FROZEN_NOT_AUTHORIZED、schedule NOT_CREATED。本手册只用于核对。完整绑定与唯一六目标/十一保留版本见[完整确认](issue307-d1-candidate20-confirmation.md)。#332 Draft、不合并，#307 OPEN。唯一工作目录 `C:/Users/Henry/.codex/worktrees/d1-lock/data-product-platform`；旧9400、d1-c17、d1-path均不作为本代入口。

1. 当前运行source `{c['Source']}`，candidate `{s['candidate_sha256']}`，bindings `{s['bindings_sha256']}`，template `{s['confirmation_sha256']}`。本代packet anchor `{anchor['packet_commit']}`仅供containment定位。准备已完成，不重跑allocate/materialize/initialize-budgets/run-non-delete/freeze，不改source/binary/candidate/config来规避漂移。旧失败/预算/pins/backup及原授权按其历史字节保留。
2. 当前唯一完整核验入口verify-six-native.py、维护入口window_six_support.py、签名入口attest-remaining-executable.py，及review06封存依赖全部按宿主索引实际SHA绑定。旧代入口不用于本代发送。复用官方MinIO SDK ParseConfig/Equals与已有Outbox→Asynq；没有平行queue、policy engine或手工业务SQL。
3. 新三预算卷归本execution/独立allocation nonce，完整挂载CID集合、local driver/options、UID/GID10001及初始空均通过；15新卷/3internal networks、source/clean/rehearsal不别名。9新及57旧备份保留（旧57含54完整与3失败partial）。本代候选准备source Core业务Commands=0（此前v28可信闭合另列）、source object PUT/DELETE=0；disabled IAM provisioning独立记录。业务Command只在独立rehearsal，不能以副本验收冒充真实H2/mixed结果。
4. 最终所有源码/候选/文档/评审/静态索引提交并通过精确HEAD PR CI后，branch-freeze.py建立exact原生lock、管理员约束、禁止fetch-merge/force/delete，并锁后核HEAD/完整保护/CI。execution-binding附录包含最终HEAD/ruleID/完整保护与全部制品SHA；缺失不得授权。锁后仅追加回执，不再push或Update branch，终态也不自动解锁。新授权必须重新绑定本代完整模板、冻结reference manifest、宿主/全部静态证据、精确发布HEAD。实际人类原文、批准模板及envelope分别保留，user_retyped_template=false。未来定时续跑仅按人类另行指令建立；本手册不创建调度。准备NOT_RECEIVED/NOT_CREATED不回写。
5. 若未来获授权，固定完整入口/Host.check/attester先核实际授权绑定的execution-binding SHA及原生fresh分支锁/最终HEAD/CI，失败早于任何Docker/SQL/签名；seal不依赖新授权或分支门禁。若未来获授权，先读本聊天最新真实人类消息，撤销/收窄优先。实时核UTC、已有terminal、当前实际HEAD/PR head/全部必要PR CI/评审线程/#332 Draft/#307 OPEN。已有terminal不再次执行；早醒只读关闭核验，过窗立即封存，不延长/平移/重开。
6. 在新的O_EXCL证据目录运行下方固定完整CLI。未来窗口内使用最终审批manifest所列final-static plan精确SHA；实际授权/调度出现后以追加phase plan核它们的真实索引，不伪造未来记录、不覆盖准备plan。语义先于permission/Docker：六case.Batch=本execution，Base.StartedAt={c['NotBefore']}、Base.ExpiresAt=CandidateUseUntil={c['NotAfter']}，六recovery.ValidUntil={recovery_end}。
7. 同一入口核实际CID/image/command/完整mount/netns/noports/no-socket/no-restart与私有文件白名单。worker UID10001、固定tmpfs/cap-drop/no-new-privileges，不挂root或host私钥；PG/native/Redis/Caddy/initializer按各自固定service descriptor，不误套worker约束。核30permission=false、3新空预算归属与完整挂载集。Docker raw wrapper仅bytes，文本严格UTF-8；类型、挂载、非空预算等失败即停，不补丁或重建budget。
8. 同一入口新鲜核三源Core REPEATABLE READ READ ONLY全关系/roles/ACL前后不变；17exact bytes/SHA/version/full namespace inventory/hold/retention、P1真实NoSuchVersion404；47 known全disabled、46sole-policy/no-groups加唯一c13 partial严格NONE/no-groups及其未分配policy官方Equals，9 absent fresh NoSuchUser。不能用raw policy SHA或2xx-only替代语义；NoSuchObjectLockConfiguration400不表示对象缺失。保存所有technical请求start/observation/真实HTTP/provider attempts与成本，金额未知NOT_ASSESSED。
9. 完整CLI关闭Source/helper后，任何重启Core/native之前先运行window_six_support.py check，要求本代实际新授权闭合且资源stopped。随后只在窗口内启动固定三Core/native，六slot及恢复期间Core保持可读；下一slot proof直接查实际immutable absence，不读host进度projection。五policy heads只读核aab0 checkpoint，不Renew、新Rights、重跑Processing/DirectData或P1。
10. root O_EXCL结果已存在先archive-root保存原字节/SHA/journal，归档失败禁止启用。仅窗口内向选中固定owner物化批准template原字节；permission仅改Enabled/真实ConfirmedAt，其他绑定/窗口/max1/max6不变。未选owner保持false。冻结active gateway核mount实际SHA，固定enable-read仅开启本代限定inspector；root bootstrap与worker物理分离，wire私钥仅wire、host私钥不挂worker。
11. 六slot按P2→T1→T2→H2A→M1A→M1B；每份最多一次、全局最多六次。每sender/wire/recovery前各自≤1分钟fresh实际拓扑，wire context≤3分钟、逐target刷新proof。预算先于Core claim，SDK无重试，UNKNOWN不重发；逐份send→disable-send→选定Outbox EventID经既有Asynq只读recover→Core absence→下一份，禁止旧四send批次、抢窗补发。
12. P2/T1/T2复用已完成业务输出/可信owner终止/handles/pin/ref闭合，只新增本execution fixture basis/批准/dispatch，旧已撤销批准/attempt不复活。H0/H1零send；case-late exit0/PID0后revoke-case只撤销Base.Enabled并私有保存原profile，owner永不重开。全部late operator/root false、enablers停止、disable-all、≤45秒fresh check-disabled、gateway实际停止sealed后才h1-proof签一次，随后≤1分钟固定close-H1记NOT_SENT，不能填布尔值或自动重签。
13. H2仅A真实上游204后才witness/late Core holds A+B/revoke/丢响应，形成CONFIRMED UNKNOWN，B永不dispatch。M1先A一次并提交absence，再B一次形成CONFIRMED UNKNOWN+C-only保护阻断checkpoint；原checkpoint复制recover-M1B/case-mixed-c并核SHA前禁止B lookup/全namespace query，随后仅B只读lookup，C永不删除。
14. 只读恢复仅冻结recover-*、选定Outbox→Asynq及独立readonly期限，最晚12:40Z；不能重启身份或延长发送，关闭IAM无法lookup则保留UNKNOWN/NOT_RUN。10:40Z起禁止新增wire/sender/enable，不使用剩余预算补发。
15. 任一非预期失败立即停止新send，固定window_six_support.py seal：关闭全部30permission/case/recovery，停sender/wire/enable owners，各组仅固定disable-all，再停/封gateway，继续关闭全部source/Core/native、恢复Redis和原business Redis至PID0。inspect失败仍尝试已知完整CID；一项失败继续其他组。归档失败关闭时仍尝试disable-all，无fresh身份读回/enabler未闭合/归档不完整/未知owner均UNCONFIRMED，不能整轮PASS。
16. seal不启动Source/enable/sender/wire，不cleanup/hold-release/bypass/release/退款，不覆盖原失败/UNKNOWN/attempt/cost/budget/pins/backup。终态和原始字节SHA索引另存本execution目录，未跑NOT_RUN，完整D1不提前接受。未来另获调度时完成、失败、过期暂停同一heartbeat，不建重复调度。

当前准备完整入口已实际通过；以下是准备plan的精确参数，每次须选尚不存在的新绝对输出目录，不能覆盖已完成readonly01。未来固定窗口内改用审批附录final-static plan及其精确SHA。审计本身不授予发送。

```powershell
python -X utf8 examples/lifecycle-d1/verify-six-native.py `
  --candidate-sha256 {s['candidate_sha256']} `
  --bindings-sha256 {s['bindings_sha256']} `
  --checker-sha256 {sha(PRIVATE/'readonly-check.exe')} `
  --evidence-audit-plan docs/acceptance/evidence/issue307-d1/remaining-candidate20/preparation-evidence-audit-plan.json `
  --evidence-audit-plan-sha256 {sha(PACKET/'preparation-evidence-audit-plan.json')} `
  --output <本工作目录内尚不存在的绝对evidence子目录>
```

当前完整入口legacy old_backups_retained=48仅为旧子门禁投影，全部57旧+9新备份SHA由历史完整门禁和本代独立读回另核，不能将旧子集字段当全量计数。

Windows Git子进程使用core.longpaths=true或仅子进程GIT_CONFIG_COUNT/KEY_n/VALUE_n，不改全局/local Git配置。源码评审、原生准备、完整只读、冻结包独立评审、精确PR HEAD CI和人类授权分别留证；本代未授权、未调度，全部发送权限关闭。最后发布HEAD CI另存发布回执，不能拿source/旧HEAD绿灯代替，也不是main CI。
'''
    write(DOCS/'issue307-d1-v30-window-runbook.md',runbook)
    print(json.dumps({'machine_lines':len(template.splitlines()),'host_binding_members':len(members),'authorization':'NOT_RECEIVED','schedule':'NOT_CREATED'}))


if __name__=='__main__':main()
