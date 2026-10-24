"""Append the actual locked HEAD to the frozen proposal, without changing HEAD."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[5]
HERE = Path(__file__).parent
BASE = HERE.parent
PUB = BASE / 'candidate22-publication01'
PACKET = BASE / 'remaining-candidate22'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate22')
load = lambda p: json.loads(p.read_bytes())
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()


def need(ok, message):
    if not ok:
        raise ValueError(message)


def write(path, text):
    with path.open('xb') as stream:
        stream.write(text.encode('utf-8'))


def main():
    path = PUB / 'branch-freeze01/execution-binding.json'
    binding = load(path)
    lock = binding['branch_lock']
    after = lock['after']
    c = load(PACKET / 'candidate.json')
    head = subprocess.check_output(['git', '-c', 'core.longpaths=true', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip()
    need(head == lock['review_head'] == after['head'], 'Locked/local HEAD differs')
    need(not subprocess.check_output(['git', '-c', 'core.longpaths=true', 'diff', '--name-only', 'HEAD'], cwd=ROOT).strip(), 'Tracked files changed after lock')
    need((PRIVATE / 'execution-binding.json').read_bytes() == path.read_bytes(), 'Public/private binding bytes differ')
    need(binding['status'] == 'FROZEN_EXECUTION_BINDING_NOT_AUTHORIZED' and binding['send_authorization'] is False
         and binding['execution'] == c['Execution'], 'Foreign or authorized binding')
    need(not (PRIVATE / 'authorized-execution01').exists()
         and not (BASE / 'delete-confirmed-v32-execution01').exists(), 'Unexpected new authority')
    need(after['ci']['head_sha'] == head and after['ci']['event'] == 'pull_request'
         and after['ci']['status'] == 'completed' and after['ci']['conclusion'] == 'success'
         and len(after['ci']['jobs']) == 13
         and all(j['conclusion'] == 'success' for j in after['ci']['jobs']), 'Final exact CI incomplete')
    need(after['draft'] and after['state'] == 'open' and not after['merged'] and after['issue_state'] == 'open', 'PR/Issue state differs')
    bsha = sha(path)
    manifest = HERE / 'approval-reference-manifest01.json'
    plan = HERE / 'final-static-evidence-audit-plan01.json'
    review = BASE / 'candidate22-artifacts-review01/independent-final-review01.json'
    complete = BASE / 'remaining-candidate22-readonly01/complete.json'
    host = HERE / 'candidate22-host-bindings01.json'
    template = (PACKET / 'confirmation-template.txt').read_bytes()
    static = (ROOT / 'docs/acceptance/issue307-d1-candidate22-confirmation.md').read_text(encoding='utf-8')
    suffix = static[static.index('## 宿主、恢复与证据绑定'):]
    refs = [path, manifest, plan, review, complete, host,
            ROOT / 'docs/acceptance/issue307-d1-candidate22-confirmation.md',
            ROOT / 'docs/acceptance/issue307-d1-v32-window-runbook.md']
    text = f'''# #307 D1 candidate22/v32 锁后完整精确确认方案

当前为 **FROZEN_NOT_AUTHORIZED**，新 execution `{c['Execution']}`。固定窗口仅2026-10-23新加坡18:00–18:40（UTC10:00≤now<10:40），只读恢复截止UTC12:40（新加坡10月23日20:40）。六份exact目标按P2→T1→T2→H2A→M1A→M1B，每份最多一次、全局最多六次；UNKNOWN不重发。P1永久排除，十一份survivor和全部其他保留限制见下方完整原文。尚无本代人类删除授权、无定时续跑，旧授权不复用。#332 OPEN/Draft/unmerged，#307 OPEN；真实DELETE=0，完整D1仍NOT_ACCEPTED。

## 最终执行与分支保护绑定

最终PR/本地HEAD `{head}`；精确该HEAD的PR CI run `{after['ci']['id']}`，attempt `{after['ci']['attempt']}`、event=pull_request、13/13 SUCCESS。这是PR CI，未合并，没有本代main CI。原生exact规则ID `{lock['rule_id']}`、branch `{lock['branch']}`，已锁后重读HEAD/完整保护/CI；原生lock、管理员约束已生效，fork-sync/force-push/delete禁止。非freeze保护与锁前一致，锁后不得推进HEAD、Update branch、merge/rebase，不自动解锁。它是准备事实，不是删除授权，也不声称原子CAS。

公开execution-binding路径 `{path.relative_to(ROOT).as_posix()}`，SHA256 **`{bsha}`**；私有同名文件逐字节一致。其制品字段固定candidate/bindings/template/source-freeze/R/reader/sender/owner，分支字段含最终HEAD、ruleID、完整REST保护和全部CI provenance。完整保护当前原文如下：

```json
{json.dumps(lock['protection'], ensure_ascii=False, indent=2)}
```

未来实际人类授权须同时引用本完整方案、全部冻结模板/宿主/审批manifest和本锁后binding SHA/最终HEAD。实际人类原文和批准机器文本分别保存，按引用时user_retyped_template=false；本方案本身不新增授权。未来执行完整入口须额外传 `--execution-binding-sha256 {bsha}`，并使用以下final-static plan精确SHA；后来真实授权/调度以追加phase plan核验，不覆盖准备plan。定时续跑只按人类另行指令建立。

| 当前审批成员 | SHA256 |
| --- | --- |
'''
    text += ''.join(f'| `{p.relative_to(ROOT).as_posix()}` | `{sha(p)}` |\n' for p in refs)
    text += f'''
审批manifest固定{load(manifest)['count']}个原准备引用，排除其自身、可变矩阵与未来授权/调度/锁后记录；后续实际授权另外固定manifest自身与本binding/本完整展示，不能把未来事实伪造进旧索引。静态方案内“最终分支锁待完成”等字样保留为提交前历史快照；本节实际锁后原始回执提供当前事实。

## 六份目标与十一份保留版本的完整机器文本

以下完整{len(template.splitlines())}行与冻结confirmation-template.txt原文一致，SHA256 `{sha(PACKET / 'confirmation-template.txt')}`，不得按相似key或旧版本猜测范围。

```text
'''
    text += template.decode('utf-8') + '```\n\n' + suffix
    text += f'''

## 执行前条件与当前验收

准备阶段已实际完成三组原生非删除备份/分离恢复/副本重跑、固定完整17版本只读、独立冻结评审和最终静态同入口audit。独立报告remaining_blockers=[]。1085已知CID均stopped/PID0/noports/norestart、30permission=false；30旧预算及3独立新空预算、66旧+9新备份原字节保留。当前新授权及schedule均不存在；这些准备证明不能替代未来授权窗口内的新鲜核验。

未来若授权，先核最新真实人类消息/UTC/本execution terminal，再核全部审批原字节、精确锁后HEAD/ruleID/完整保护/必要CI/评审线程/#332 Draft/#307 OPEN。固定入口新鲜核case/window、全拓扑与预算、三源Core/native/17版本/完整库存保护/IAM及全部备份，自动关闭后先Host.check，再允许窗口内有限启用。每份严格send→disable-send→选定Outbox→既有Asynq只读recover→Core immutable absence→下一份。每sender/wire/recovery有各自≤1分钟fresh host proof，wire context≤3分钟且逐target刷新，预算先于Core claim，SDK无重试。H0/H1零send，H2及M1有限顺序、checkpoint先于B lookup等全部沿冻结手册。

任一非预期失败或10:40Z到时即停止新增enable/wire/sender，逐组固定seal；归档或inspect失败仍继续已知CID的全部关闭。无fresh身份读回/enabler未闭合/归档不完整为UNCONFIRMED，不报整轮PASS。无cleanup/hold-release/bypass/release/退款，不覆盖旧失败/UNKNOWN/attempt/成本/预算/pins；不重发UNKNOWN、不延长/平移/重开窗口。只读恢复截止12:40Z不授予重新启用身份或发送。
'''
    proposal = ROOT / 'docs/acceptance/issue307-d1-candidate22-final-confirmation01.md'
    write(proposal, text)
    checkpoint = dict(status='PASS_C22_PREPARATION_FINAL_CI_AND_NATIVE_BRANCH_FREEZE_NOT_AUTHORIZED',
        execution=c['Execution'], final_head=head, ci_run=after['ci']['id'], ci_event='pull_request',
        ci_jobs=13, rule_id=lock['rule_id'], execution_binding_sha256=bsha,
        final_proposal=str(proposal.relative_to(ROOT)), final_proposal_sha256=sha(proposal),
        manifest_sha256=sha(manifest), final_static_audit_plan_sha256=sha(plan),
        candidate_sha256=sha(PACKET/'candidate.json'), bindings_sha256=sha(PACKET/'bindings.json'),
        template_sha256=sha(PACKET/'confirmation-template.txt'), targets=6, survivors=11,
        not_before=c['NotBefore'], not_after=c['NotAfter'], authorization='NOT_RECEIVED',
        schedule='NOT_CREATED', old_authorization_reused=False, native_delete=0,
        full_D1='NOT_ACCEPTED', PR332='OPEN_DRAFT_UNMERGED', Issue307='OPEN')
    write(PUB / 'final-checkpoint01.json', json.dumps(checkpoint, ensure_ascii=False, indent=2) + '\n')
    matrix = f'''# #307 D1 candidate22/v32 锁后验收追加记录

本记录追加到提交前[v32矩阵](issue307-d1-v32-preparation.md)，不覆盖其历史PENDING字段。

| 验收项 | 当前事实 |
| --- | --- |
| execution/窗口 | `{c['Execution']}`；2026-10-23新加坡18:00–18:40 |
| 最终PR HEAD | `{head}` |
| 精确该HEAD PR CI | run{after['ci']['id']}，pull_request，13/13 SUCCESS；不是main CI |
| 原生分支冻结 | PASS；rule `{lock['rule_id']}`，管理员约束、禁fork-sync/force/delete；完整保护锁后读回 |
| execution-binding | `{bsha}`；公开/私有同字节，锁后不再推进HEAD |
| 六份目标/十一survivor | 冻结一致；P1永久排除 |
| 完整17版本原生/非删除/独立评审/静态audit | PASS；原始证据与0有效阻断保留 |
| 当前授权/调度/DELETE | NOT_RECEIVED / NOT_CREATED / 0，旧授权不复用 |
| 完整D1/#332/#307 | NOT_ACCEPTED / OPEN-Draft-unmerged / OPEN |

完整新方案：[锁后完整精确确认](issue307-d1-candidate22-final-confirmation01.md)。未来发送仍需新的人类精确授权与固定窗口内全部新鲜前置；所有保留限制不变。
'''
    write(ROOT / 'docs/acceptance/issue307-d1-v32-postlock-matrix01.md', matrix)
    print(json.dumps({'status': checkpoint['status'], 'head': head,
        'execution_binding_sha256': bsha, 'proposal_sha256': sha(proposal), 'authority': False}))


if __name__ == '__main__':
    main()
