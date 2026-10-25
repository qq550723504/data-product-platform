"""Terminalize this failed exact-head gate; never permit a retry or move window."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import subprocess
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent;PARENT=OUT.parent;BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19\authorized-execution01')
def need(ok,message):
    if not ok:raise ValueError(message)
def load(p):return json.loads(p.read_bytes())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(p,v):
    with p.open('xb') as f:f.write((json.dumps(v,ensure_ascii=False,indent=2)+'\n').encode())
def write(p,raw):
    with p.open('xb') as f:f.write(raw)
a=load(PARENT/'human-authorization.json');failure=load(OUT/'failure-head-drift01.json')
commit=load(OUT/'actual-head-commit01.stdout.json');closed=load(OUT/'post-seal-readback.json');seal=load(OUT/'failure-seal01.stdout.json')
need(commit['sha']==failure['actual_pr_head'] and [x['sha'] for x in commit['parents']]==[failure['expected_head'],'a25ebac1c930d749daa085ef63ca3db8f0e7d6b8'],'Actual upstream merge cause differs')
need(closed['owners_closed']==820 and closed['new_budgets_empty']==3 and closed['old_budgets_preserved']==21 and closed['backups_verified']==57 and closed['heartbeat_deleted'] is True,'Containment/retention readback incomplete')
need(seal['all_steps_confirmed'] is False and closed['fresh_IAM_disable']=='UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START','Do not relabel incomplete fresh IAM evidence as PASS')
need(not (CONTROL/'terminal.json').exists(),'Execution already terminal; never overwrite')
need(subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()==a['final_approved_head'] and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Local source/HEAD modified')
for row in a['approved_reference_artifacts']:
    path=ROOT/row['path']
    need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'Original approved evidence overwritten')
for index_name in ('authorization-evidence-index01.json','prewindow-evidence-index01.json','schedule-authority-index01.json','schedule-evidence-index01.json'):
    index=load(PARENT/index_name);root=ROOT if index_name in ('prewindow-evidence-index01.json','schedule-evidence-index01.json') else PARENT
    need(len(index['files'])==index['count'],'Historical index count drift')
    for name,facts in index['files'].items():
        path=root/name
        need(sha(path)==facts['sha256'] and path.stat().st_size==facts['bytes'],'Original authorization/schedule evidence changed')
at=dt.datetime.now(dt.timezone.utc).isoformat()
cause={'status':'CONFIRMED_FROZEN_PR_HEAD_DRIFT_BEFORE_NATIVE_PRECHECK','at':at,'expected_head':failure['expected_head'],'actual_pr_head':failure['actual_pr_head'],'observed_remote_commit_time':commit['commit']['committer']['date'],'commit_message':commit['commit']['message'],'parents':[x['sha'] for x in commit['parents']],'changed_files':len(commit['files']),'changed_paths':[x['filename'] for x in commit['files']],'gate_semantics':'Human authorization and frozen runbook require both local and PR HEAD to equal caab46da. Actual PR branch merged main at 03:11:35Z after the 03:09Z scheduling receipt. Old approved-head CI success cannot authorize another HEAD.','impact':'Source/runtime paths and migrations changed in the PR; this is not an append-only receipt-only change. Local frozen candidate/source/binaries stayed unchanged.','boundary_gap':'The frozen execution depends on a PR head that remained writable. Current gates detect drift and stop; no branch freeze prevents later main-to-PR updates.','next_bounded_work':'First settle frozen execution source/PR head and update ownership, then review any replacement plan. Do not ignore drift, rewrite approval, reset budgets, create a new candidate or choose another window under this old grant.'}
save(OUT/'head-drift-root-cause01.json',cause)
matrix=ROOT/'docs/acceptance/issue307-d1-v29-window01.md'
write(matrix,f"""# #307 D1 candidate19/v29 固定窗口实际结果

记录：{at}。execution {a['execution']}。状态：FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE。完整D1未接受，#332保持OPEN/Draft/unmerged，#307保持OPEN。

2026-10-22新加坡14:00窗口唤醒后，第一轮真实GitHub门禁发现PR当前HEAD 17c5248a868376603ae9c3951123c545f2f7e56f，授权/冻结要求caab46da8761e71e1d80d0c0931707a410c13a4d。GitHub原始commit回执证实03:11:35Z（新加坡11:11:35）将main合入PR分支，父提交为授权HEAD与main a25ebac1c930d749daa085ef63ca3db8f0e7d6b8，涉及{len(commit['files'])}个文件，包括运行代码和migrations 70/71。不能用旧HEAD绿色PR CI替代新HEAD门禁，也不能在窗口内修改冻结限制继续执行。

| 验收项 | 实际结果 |
| --- | --- |
| 当前授权原文/924批准引用 | 原字节匹配；不复用旧授权 |
| 精确HEAD前置 | FAIL；本地仍为授权caab46da，远端PR为17c5248a |
| PR/Issue状态 | #332 OPEN/Draft/unmerged；#307 OPEN |
| 旧批准HEAD CI | pull_request成功，仅为旧HEAD事实；未冒充新HEAD或main CI |
| 完整窗口native/Core/IAM/17版本核验 | NOT_RUN；在启动源环境前停止 |
| P2/T1/T2/H2A/M1A/M1B | 全部NOT_RUN；实际DELETE=0/6，无新增dispatch/provider invocation |
| 新权限启用/签发送proof/source启动/Core Commands | 全部0 |
| 固定seal | 已对全部组执行；103步骤中100确认、3个disable-all因native原已停止而无fresh IAM读回 |
| IAM关闭新鲜证明 | UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START；不启动原生服务补查，不声称整轮PASS |
| 实际物理封存 | 820固定已知CID全部stopped/PID0/noports/norestart；30permissionfalse；6case/6recovery撤销；3gateway sealed |
| 新旧预算 | 21旧预算tar SHA与完整挂载CID集合不变；3新预算SHA不变且仍为空，未消费或退款 |
| 备份/原证据 | 57备份SHA不变；924批准引用及原授权/调度索引不变；seal原profile字节在私有seal-history追加保存 |
| pins | 本次无Core启动或Command，不释放/改写旧pins；fresh Core pin查询NOT_RUN |
| 原生库存 | 本轮未重新读取；不把历史17版本结果冒充窗口内fresh |
| 调度 | 307-d1-candidate19-v29已删除，实际数据库与TOML均无记录；不补发、不新建调度 |
| 窗口/保留限制 | 不延长、平移、重开、UNKNOWN重发；cleanup=0、hold-release=0、bypass=0 |

已保存原始失败tool回执、6项GitHub fresh响应、actual merge commit、全部固定seal结果、物理关闭/24预算/57备份读回及哈希索引。原失败和旧预算/pins保留，原授权/准备矩阵及其历史NOT_CREATED/NOT_RECEIVED不回写。

当前设计边界是：冻结执行依赖的PR HEAD仍可被更新，门禁只能发现漂移后停止。下一步应先明确冻结期间PR分支的更新归属与执行绑定，再评审替代方案；本次不自动生成新候选或窗口，不复用此授权。

[实际失败与封存](evidence/issue307-d1/delete-confirmed-v29-execution01/window01/post-seal-readback.json) · [根因证据](evidence/issue307-d1/delete-confirmed-v29-execution01/window01/head-drift-root-cause01.json)
""".encode())
files=sorted(p for p in OUT.rglob('*') if p.is_file())+[matrix]
save(OUT/'evidence-index.json',{'status':'C19_V29_WINDOW01_FAILED_HEAD_GATE_EVIDENCE_INDEX','execution':a['execution'],'count':len(files),'files':{p.relative_to(ROOT).as_posix():{'sha256':sha(p),'bytes':p.stat().st_size} for p in files}})
terminal={'status':'FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE','at':at,'execution':a['execution'],'profile':'candidate19/v29','terminal':True,'original_authorization_sha256':sha(PARENT/'human-authorization.json'),'window_not_before':'2026-10-22T06:00:00Z','window_not_after_exclusive':'2026-10-22T06:40:00Z','expected_local_and_PR_head':failure['expected_head'],'actual_local_head':a['final_approved_head'],'actual_PR_head':failure['actual_pr_head'],'root_cause':'PR branch merged main after frozen authorization and scheduling; exact HEAD gate failed before native or source actions','root_cause_sha256':sha(OUT/'head-drift-root-cause01.json'),'full_native_preflight':'NOT_RUN','new_source_start':0,'new_permission_enable':0,'new_proof_signatures':0,'new_provider_invocations':0,'new_Core_commands':0,'physical_provider_DELETE_attempts':0,'new_native_delete':0,'targets':{stage:'NOT_RUN' for stage in ('P2','T1','T2','H2A','M1A','M1B')},'new_budgets_empty':3,'old_budgets_preserved':21,'backups_preserved_SHA':57,'owners_closed':820,'permission_false':30,'cases_revoked':6,'recoveries_revoked':6,'gateway_sealed':3,'seal_all_steps_confirmed':False,'fresh_IAM_disable':'UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START','native_or_Core_restarted_to_recheck':False,'old_pins':'NO_WRITE_NO_RELEASE; FRESH_QUERY_NOT_RUN','all_failure_evidence_retained':True,'heartbeat':'DELETED','automation_id':'307-d1-candidate19-v29','retry_allowed':False,'old_authorization_reusable':False,'cleanup':0,'hold_release':0,'bypass':0,'PR332':'OPEN_DRAFT_UNMERGED','Issue307':'OPEN','full_D1':'NOT_ACCEPTED','evidence_index_sha256':sha(OUT/'evidence-index.json'),'acceptance_matrix_sha256':sha(matrix),'provider_amount':'NOT_ASSESSED; NO_NEW_NATIVE_PROVIDER_INVOCATION'}
save(OUT/'terminal.json',terminal);save(CONTROL/'terminal.json',terminal)
checkpoint={'status':terminal['status'],'execution':a['execution'],'terminal_sha256':sha(OUT/'terminal.json'),'private_terminal_equal':(CONTROL/'terminal.json').read_bytes()==(OUT/'terminal.json').read_bytes(),'evidence_index_sha256':sha(OUT/'evidence-index.json'),'matrix_sha256':sha(matrix),'heartbeat_deleted':True,'new_native_delete':0,'full_D1':'NOT_ACCEPTED'}
save(OUT/'final-checkpoint.json',checkpoint)
print(json.dumps(checkpoint))

