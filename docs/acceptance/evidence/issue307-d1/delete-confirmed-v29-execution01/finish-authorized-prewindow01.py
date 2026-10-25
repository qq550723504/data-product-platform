"""Save the bounded authorization/prewindow matrix; no scheduling or execution."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import subprocess
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19\authorized-execution01')
def need(ok,message):
    if not ok:raise ValueError(message)
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path):return json.loads(path.read_bytes())
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
def save(path,value):write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
a=load(OUT/'human-authorization.json')
h=load(OUT/'authorized-prewindow-check01.json')
g=load(OUT/'github-checkpoint01.json')
audit=load(OUT/'authorized-static-audit01/evidence-audit.json')
need(h['status']=='PASS_PREWINDOW_CLOSED' and h['actual_host_check']['execution']==a['execution'],'Actual Host.check incomplete')
gate=h['actual_host_check']['host_gate']
need(gate['permissions_false']==30 and gate['budgets_empty']==3 and gate['owners_closed']==86,'Closed host state differs')
need(g['head']==a['final_approved_head'] and g['jobs_success']==13 and g['unresolved_threads']==0 and g['draft'] and not g['merged'] and g['issue_state']=='OPEN','GitHub checkpoint differs')
need(audit['status']=='PASS_EXPLICIT_EVIDENCE_INDEX_AUDIT' and audit['members_verified']==5470,'Actual grant offline audit incomplete')
head=subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()
need(head==a['final_approved_head'] and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Tracked source/HEAD drift')
at=dt.datetime.now(dt.timezone.utc).isoformat()
matrix=ROOT/'docs/acceptance/issue307-d1-v29-execution01.md'
text=f"""# #307 D1 candidate19/v29 授权后窗口前矩阵

记录时间：{at}。execution {a['execution']}。本次新的人类按引用授权已收到，原文432 UTF-8 bytes（含末尾LF），SHA256 {a['user_message_sha256']}。批准模板另存，user_retyped_template=false；不复用旧授权。

唯一发送窗口：2026-10-22 新加坡14:00≤now<14:40（UTC06:00–06:40）；只读恢复最晚16:40（08:40Z），不授予重新启用或延长发送。顺序P2→T1→T2→H2A→M1A→M1B，每份max1、全局max6，UNKNOWN不重发。6目标/11保留版本及全部绑定以冻结模板为准，P1永久排除。

| 项目 | 本次事实 |
| --- | --- |
| 直接人类授权 | RECEIVED_EXPLICIT_USER_AUTHORIZATION_BY_REFERENCE；实际人类原文与机器模板分开留证 |
| 924项批准引用及私有envelope/host bindings | 原字节核对通过 |
| 固定完整入口离线审计 | PASS；9 indexes / 5470 references；files映射与members列表显式区分 |
| 实际冻结Host.check | PASS_PREWINDOW_CLOSED；30权限false、86本代owners关闭、3新预算为空、6case/6recovery精确 |
| 保留备份 | 本次固定门禁9新+48旧SHA匹配；旧预算与pins未修改，旧失败保持历史事实 |
| 当前HEAD与PR CI | {head}；run39710200629 / pull_request / 13成功；不是main CI |
| PR/Issue/评审线程 | #332 OPEN/Draft/unmerged；#307 OPEN；未解决线程0 |
| 窗口内fresh Core/native/IAM/17版本全量核验 | NOT_RUN；必须在固定窗口内通过，不能用本次关闭检查替代 |
| 启用、签发送proof、source/provider I/O、DELETE | 本次均0 |
| 独立准备评审 | 原冻结报告PASS；不声称本次新增授权/调度另经独立复评 |
| 新定时续跑 | NOT_CREATED_REQUIRES_SEPARATE_HUMAN_REQUEST；删除授权无需重复 |
| 完整D1 | NOT_ACCEPTED；六目标本次尚未执行 |

不修改candidate/source/binary/config，不重跑prepare/freeze、重建资源或预算；保留新旧预算、旧pins、57备份、其他环境及全部失败/UNKNOWN/attempt/成本事实。cleanup=0、hold-release=0、bypass=0。失败或过窗立即固定seal，不延长、平移、重开或补发。原准备NOT_RECEIVED/NOT_CREATED、旧失败与旧授权不覆盖。

后续如另获调度指令，只建立本execution固定日期的一次续跑；开窗先实时核UTC、终态、当前HEAD/CI/Draft/Open/线程，运行冻结完整CLI的新phase审计计划与fresh Core/native/IAM/17版本/拓扑/预算检查，再按冻结有限顺序执行。已有terminal不再执行。任何非预期失败先封存，实际未跑NOT_RUN，完成/失败/过期停同一调度。

[授权原始回执](evidence/issue307-d1/delete-confirmed-v29-execution01/human-authorization.json) · [实际窗口前核验](evidence/issue307-d1/delete-confirmed-v29-execution01/authorized-prewindow-check01.json) · [窗口手册](issue307-d1-v29-window-runbook.md)
"""
write(matrix,text.encode())
files=sorted(p for p in OUT.rglob('*') if p.is_file())
index={'status':'C19_V29_AUTHORIZED_PREWINDOW_EVIDENCE_INDEX','execution':a['execution'],'count':len(files)+1,'files':{p.relative_to(ROOT).as_posix():{'sha256':sha(p),'bytes':p.stat().st_size} for p in [*files,matrix]}}
save(OUT/'prewindow-evidence-index01.json',index)
checkpoint={'status':'PASS_C19_V29_ACTUAL_AUTHORIZATION_AND_PREWINDOW_CLOSED','at':at,'execution':a['execution'],'head':head,'human_authorization_sha256':sha(OUT/'human-authorization.json'),'user_message_sha256':a['user_message_sha256'],'authorization_envelope_sha256':a['authorization_envelope_sha256'],'approved_reference_manifest_sha256':a['approved_reference_manifest_sha256'],'approved_host_bindings_sha256':a['additional_host_bindings_sha256'],'approved_reference_count':a['approved_reference_artifact_count'],'authorized_evidence_audit_plan_sha256':sha(OUT/'authorized-evidence-audit-plan01.json'),'prewindow_evidence_index_sha256':sha(OUT/'prewindow-evidence-index01.json'),'matrix_sha256':sha(matrix),'permissions_false':30,'owners_closed':86,'new_budgets_empty':3,'backups_sha_verified':57,'PR332':'OPEN_DRAFT_UNMERGED','Issue307':'OPEN','PR_CI':{'run':39710200629,'event':'pull_request','success':13,'main_ci_claimed':False},'schedule':'NOT_CREATED_REQUIRES_SEPARATE_HUMAN_REQUEST','fresh_window_native_preflight':'NOT_RUN_REQUIRED','native_delete':0,'source_start':0,'provider_io':0,'permission_enable':0,'proof_signatures':0,'full_D1':'NOT_ACCEPTED','execution_terminal':False}
save(OUT/'final-checkpoint01.json',checkpoint)
save(CONTROL/'authorization-prewindow-checkpoint01.json',checkpoint)
print(json.dumps(checkpoint))

