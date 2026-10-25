"""Append actual scheduler receipts without changing the prior authorization index."""
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
a=load(OUT/'human-authorization.json');s=load(OUT/'schedule-human-authority.json');r=load(OUT/'scheduled-local-readback01.json')
need(r['status']=='PASS_ACTUAL_SINGLE_HEARTBEAT_FIXED_NEXT_RUN_AND_PREWINDOW_CLOSED' and r['automation']['id']=='307-d1-candidate19-v29' and r['next_run_utc']=='2026-10-22T06:00:00+00:00' and r['automation_count_for_execution']==1 and r['scheduler_runs']==0,'Actual schedule not exact')
need(sha(OUT/'human-authorization.json')=='c4e37256edd4138f5d443287373efb6b0e1810c559fb0899caf014762840d858' and sha(OUT/'scheduled-prompt01.txt')==r['actual_prompt_sha256'],'Prior authority or actual prompt changed')
for index_name in ('authorization-evidence-index01.json','prewindow-evidence-index01.json','schedule-authority-index01.json'):
    index=load(OUT/index_name)
    root=ROOT if index_name=='prewindow-evidence-index01.json' else OUT
    need(index['count']==len(index['files']),'Original index member count differs')
    for name,facts in index['files'].items():
        path=root/name
        need(sha(path)==facts['sha256'] and path.stat().st_size==facts['bytes'],'Existing indexed bytes changed: '+name)
for artifact in a['approved_reference_artifacts']:
    path=ROOT/artifact['path']
    need(sha(path)==artifact['sha256'] and path.stat().st_size==artifact['bytes'],'Approved reference byte drift')
need(sha(CONTROL/'authorization-envelope.txt')==a['authorization_envelope_sha256'] and sha(CONTROL/'approved-host-bindings.txt')==a['additional_host_bindings_sha256'] and sha(CONTROL/'approved-reference-manifest.json')==a['approved_reference_manifest_sha256'],'Private original receipt drift')
need((CONTROL/'schedule-human-reply-verbatim.txt').read_bytes()==(OUT/'schedule-human-reply-verbatim.txt').read_bytes() and sha(OUT/'schedule-human-reply-verbatim.txt')==s['actual_wrapper_sha256'],'Actual schedule human reply drift')
head=subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()
need(head==a['final_approved_head'] and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Tracked source/HEAD changed')
at=dt.datetime.now(dt.timezone.utc).isoformat()
matrix=ROOT/'docs/acceptance/issue307-d1-v29-schedule01.md'
write(matrix,f"""# #307 D1 candidate19/v29 固定窗口续跑回执

记录：{at}。execution {a['execution']}。此为授权后新增调度事实；原准备/授权矩阵的NOT_CREATED保留当时事实，不覆盖。

| 项目 | 当前核对结果 |
| --- | --- |
| 删除授权 | 人类直接按引用授权已记录；924项批准引用保持原字节；不复用旧授权 |
| 单独调度授权 | 人类回答“安排固定窗口续跑（推荐）”；真实wrapper751 UTF-8 bytes另存 |
| 实际automation | 307-d1-candidate19-v29；heartbeat；ACTIVE；目标为本聊天 |
| 实际下次运行 | 2026-10-22T06:00:00Z = 新加坡2026-10-22 14:00；next_run与nominal一致 |
| 数量及历史 | 本execution仅1调度；已运行0次；旧07/10/11/12/15/16/17保持PAUSED，旧18不恢复 |
| 固定窗口 | 仅2026-10-22新加坡14:00≤now<14:40，过窗不延长、平移、重开或补发 |
| 追加phase索引 | 固定完整入口audit-only通过；10显式indexes/5473references |
| actual prompt | 与批准创建参数、TOML及实际SQLite prompt逐字一致 |
| 当前关闭状态 | 820个固定已知CID stopped/PID0/noports/norestart，30permission=false，worker无提前物化批准文本 |
| 新预算与保留证据 | 03:01Z固定Host.check证明3新预算为空、9新+48旧backupSHA匹配；本轮调度不消费预算或修改pins |
| HEAD/PR CI | {head}；已核13/13 pull_request成功，不是main CI；窗口内必须再核实时精确HEAD/线程/CI |
| 状态约束 | #332 OPEN/Draft/unmerged；#307 OPEN |
| 尚待执行 | 窗口内fresh Core/native/IAM/17版本、拓扑及预算完整核验NOT_RUN；本轮source_start/provider/DELETE均0 |
| 停止规则 | 完成、失败或过期暂停本同一heartbeat；已有terminal只读报告，不重复执行 |
| 完整D1 | NOT_ACCEPTED |

六目标仅P2→T1→T2→H2A→M1A→M1B，max1each/max6total，UNKNOWN不重发；P1永久排除。保留11survivor、其他环境、全部备份、新旧预算、旧pins及失败/UNKNOWN/attempt/成本事实。cleanup=0、hold-release=0、bypass=0。每slot发送关闭、选定Outbox→Asynq只读恢复并提交实际Core absence后才能下一slot；H0/H1零send，H2/M1严格冻结顺序。

原授权索引7成员、窗口前索引34成员、冻结候选/source/binary/config不变。调度prompt和实际TOML快照后存，不加入此前授权审计plan构造自引用。TOML快照是本次创建事实，未来按指令暂停后不再要求实际TOML等于创建时SHA。准备独立评审PASS不冒充新增授权/调度已独立复评。

[实际调度读回](evidence/issue307-d1/delete-confirmed-v29-execution01/scheduled-local-readback01.json) · [调度授权原文与绑定](evidence/issue307-d1/delete-confirmed-v29-execution01/schedule-human-authority.json) · [固定窗口手册](issue307-d1-v29-window-runbook.md)
""".encode())
names=['actual-app-schedule-source01.json','record-schedule-authority01.py','schedule-human-authority.json','schedule-human-reply-verbatim.txt','schedule-human-answer-verbatim.txt','schedule-authority-index01.json','scheduled-evidence-audit-plan01.json','scheduled-static-audit01.stdout.json','scheduled-static-audit01.stderr.txt','scheduled-static-audit01/evidence-audit.json','prepare-schedule-prompt01.py','scheduled-prompt01.txt','schedule-create-arguments01.json','scheduled-create-result01.json','scheduled-view-result01.json','automation-toml-snapshot01.txt','verify-scheduled-readback01.py','schedule-cold-owners01.json','scheduled-local-readback01.json','finish-schedule-evidence01.py']
files=[OUT/name for name in names]+[matrix]
save(OUT/'schedule-evidence-index01.json',{'status':'C19_V29_FIXED_SCHEDULE_EVIDENCE_INDEX','execution':a['execution'],'count':len(files),'files':{p.relative_to(ROOT).as_posix():{'sha256':sha(p),'bytes':p.stat().st_size} for p in files}})
checkpoint={'status':'SCHEDULED_EXACT_FIXED_WINDOW_SINGLE_RUN_PREWINDOW_CLOSED','observed_at':at,'execution':a['execution'],'automation_id':'307-d1-candidate19-v29','kind':'heartbeat','status_actual':'ACTIVE','target_thread_id':a['thread_id'],'next_run_utc':'2026-10-22T06:00:00Z','next_run_singapore':'2026-10-22T14:00:00+08:00','window_end_exclusive_utc':'2026-10-22T06:40:00Z','readonly_recovery_end_utc':'2026-10-22T08:40:00Z','matching_automations':1,'prior_schedules_paused':7,'old18_absent':True,'scheduled_runs_so_far':0,'authorization_sha256':sha(OUT/'human-authorization.json'),'schedule_authority_sha256':sha(OUT/'schedule-human-authority.json'),'schedule_authority_index_sha256':sha(OUT/'schedule-authority-index01.json'),'scheduled_audit_plan_sha256':sha(OUT/'scheduled-evidence-audit-plan01.json'),'scheduled_prompt_sha256':sha(OUT/'scheduled-prompt01.txt'),'saved_config_snapshot_sha256':sha(OUT/'automation-toml-snapshot01.txt'),'schedule_evidence_index_sha256':sha(OUT/'schedule-evidence-index01.json'),'actual_schedule_readback_sha256':sha(OUT/'scheduled-local-readback01.json'),'approval_references_924_unchanged':True,'original_authorization_index_7_members_unchanged':True,'original_prewindow_index_34_members_unchanged':True,'typed_indexes':10,'typed_member_references':5473,'HEAD':head,'tracked_modifications':0,'permissions_false':30,'known_cids_closed':820,'budget_fresh_check':'3 EMPTY AS OF 03:01Z FROZEN HOST.CHECK; RECHECK AT WINDOW','new_native_preflight':'NOT_RUN','new_source_start':0,'new_provider_io':0,'new_native_delete':0,'PR332':'OPEN_DRAFT_UNMERGED','Issue307':'OPEN','D1':'NOT_ACCEPTED','old_authorization_reused':False,'on_terminal':'PAUSE_THIS_SAME_HEARTBEAT; NO_EXTEND_SHIFT_REOPEN_RESEND_OR_DUPLICATE_SCHEDULE'}
save(OUT/'schedule-final-checkpoint01.json',checkpoint)
save(CONTROL/'schedule-final-checkpoint01.json',checkpoint)
print(json.dumps(checkpoint))

