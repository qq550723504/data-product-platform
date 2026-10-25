"""Append this crosscheck's current facts; never change frozen HEAD or authority."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
OUT=Path(__file__).parent
BASE=OUT.parent
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate21')
load=lambda p:json.loads(p.read_bytes())
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def need(ok,message):
    if not ok:raise ValueError(message)


def save(path,value):
    with path.open('xb') as stream:stream.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())


def main():
    files=load(OUT/'files03/result.json'); live=load(OUT/'live/result.json'); host=load(OUT/'host/result.json')
    need(files['status']=='PASS_C21_V31_FULL_TEXT_AND_ALL_FROZEN_BINDINGS_CROSSCHECK'
         and live['status']=='PASS_FRESH_C21_HEAD_NATIVE_LOCK_FULL_PROTECTION_CI_AND_THREADS'
         and host['status']=='PASS_FRESH_FIXED_C21_CLOSED_HOST_GATE','Required current checks incomplete')
    prior=load(BASE/'candidate21-planning01/final-cold-inventory01.json')
    ids=sorted({row['cid'] for row in prior['owners']});need(len(ids)==996,'Known owner union differs')
    owners=[]
    for n in range(0,len(ids),64):
        result=subprocess.run(['docker','inspect',*ids[n:n+64]],capture_output=True,check=True,timeout=30)
        for x in json.loads(result.stdout):
            need(not x['State']['Running'] and x['State']['Pid']==0 and x['HostConfig']['RestartPolicy']['Name']=='no'
                 and not x['HostConfig'].get('PortBindings') and not any((x['NetworkSettings'].get('Ports') or {}).values()),'Known owner unexpectedly active/exposed/restarting')
            owners.append(dict(cid=x['Id'],running=False,PID=0,restart='no',noports=True))
    need(len(owners)==len({x['cid'] for x in owners})==996,'Observed known membership differs')
    # Actual Docker Config.Env is neither published nor retained here.
    save(OUT/'fresh-996-closed.json',dict(status='PASS_FRESH_ALL_996_KNOWN_CIDS_CLOSED',
         at=dt.datetime.now(dt.timezone.utc).isoformat(),count=996,owners=owners,source_start=0,native_delete=0))
    run_records=[]
    for path in (OUT/'live').glob('*-stdout.json'):
        data=load(path)
        if isinstance(data,dict) and isinstance(data.get('workflow_runs'),list):
            run_records += [x for x in data['workflow_runs'] if x['path']=='.github/workflows/ci.yml']
    run=max(run_records,key=lambda x:x['id'])
    need(run['head_sha']==files['final_head'] and run['status']=='completed' and run['conclusion']=='success' and run['event']=='pull_request','Fresh CI differs')
    now=dt.datetime.now(dt.timezone.utc)
    start=dt.datetime(2026,10,22,14,tzinfo=dt.timezone.utc);end=start+dt.timedelta(minutes=40)
    need(now<start,'New authorization crosscheck completed too late for selected future window')
    need(not (PRIVATE/'authorized-execution01').exists() and not (BASE/'delete-confirmed-v31-execution01').exists()
         and not Path('C:/Users/Henry/.codex/automations/307-d1-candidate21-v31').exists(),'Unexpected authority/schedule')
    head=subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD'],cwd=ROOT).decode().strip()
    need(head==files['final_head'] and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD'],cwd=ROOT).strip(),'Locked checkout changed')
    result=dict(status='PASS_C21_V31_EXACT_CONFIRMATION_CROSSCHECK_NOT_AUTHORIZED',
        at=now.isoformat(),execution=files['execution'],profile='candidate21/v31',final_head=head,
        proposal_sha256=files['proposal_sha256'],execution_binding_sha256=files['execution_binding_sha256'],
        rule_id=live['rule_id'],native_lock='FRESH_UNCHANGED',full_native_protection='FRESH_EXACT_MATCH',
        ci_run=run['id'],ci_attempt=run['run_attempt'],ci_event='pull_request',ci_jobs=13,unresolved_threads=0,
        source_runtime='514843da1a143fba537c1668483fe95800ddd992',host_source='cef8a7f32133400f34ed2b1be0f2f4b635350190',
        displayed_machine_exact=True,machine_lines=137,public_binding_rows=82,original_build_members=12,
        static_indexes=8,static_references=6366,approval_manifest_members=1102,publication_members=255,
        targets=6,survivors=11,P1='PERMANENTLY_EXCLUDED',known_owners_closed=996,
        permissions_false=30,new_budgets_empty=3,old_budgets_retained=27,backup_hashes_retained=66,
        not_before='2026-10-22T14:00:00Z',not_after='2026-10-22T14:40:00Z',
        Singapore_window='2026-10-22 22:00-22:40 +08:00',recovery_until='2026-10-22T16:40:00Z',
        window_state='NOT_OPEN_YET',authorization='NOT_RECEIVED',schedule='NOT_CREATED',
        execution_preconditions='NOT_RELEASED_REQUIRE_NEW_HUMAN_AUTHORITY_AND_WINDOW_FRESH_FULL_PREFLIGHT',
        window_fresh_Core_native_inventory_IAM='NOT_RUN_THIS_CROSSCHECK_SOURCE_NOT_STARTED',
        host_attestations_for_send='NOT_SIGNED',native_delete=0,source_start=0,enable=0,
        full_D1='NOT_ACCEPTED',PR332='OPEN_DRAFT_UNMERGED',Issue307='OPEN',
        old_authorization_reused=False,cleanup=0,hold_release=0,bypass=0,
        preserved_checker_failures={'files01':'Crosscheck field offsets; corrected in02',
                                   'files02':'Initial-allocation permission shape used instead of frozen operator/root schemas; corrected strictly in03'},
        frozen_source_packet_template_binaries_HEAD_changed=False)
    save(OUT/'result.json',result)
    rows=load(OUT/'files03/exact-version-crosscheck.json')['versions']
    target_table='| 目标 | exact namespace/key@version |\n| --- | --- |\n'
    stages=['P2','T1','T2','H2A','M1A','M1B']
    selected=[x for x in rows if x['role']=='删除目标']
    for stage,row in zip(stages,selected):
        cp=row['copy'];target_table+=f'| {stage} | `{cp["Namespace"]}/{cp["Key"]}@{cp["ProviderVersion"]}` |\n'
    report=f'''# #307 D1 candidate21/v31 完整确认逐项核对

本轮结论 **PASS，冻结绑定无漂移，当前方案核对范围0阻断**。这是新授权前的只读核对，不是删除授权或完整D1验收。

execution `{result['execution']}`；最终HEAD `{head}`；固定新加坡时间2026-10-22 22:00–22:40，UTC14:00≤now<14:40；恢复截止UTC16:40，即新加坡10月23日00:40。

| 核对项 | 当前实际结论 |
| --- | --- |
| 02完整方案及聊天展示 | proposal SHA `{result['proposal_sha256']}`；聊天137行机器文本/全部94表行与冻结原件一致 |
| 冻结候选、R/P、原始构建、完整引用 | PASS；R769/P2247源成员原字节/规范LF；12原构建；82公开绑定；1102审批成员；8索引6366引用；255发布成员 |
| 六份目标/十一保留版本 | namespace/key/version/copy/DV/bytes/SHA/plan/hash/hold/retention逐项一致，P1永久排除 |
| 最终HEAD/原生分支保护 | GitHub实时读回HEAD一致，rule `{result['rule_id']}`及完整保护与冻结收据相同；lock/admin=true，fork-sync/force/delete=false |
| execution-binding | public/private同字节，SHA `{result['execution_binding_sha256']}`；全部制品字段一致 |
| 精确HEAD PR CI/评审线程 | run{run['id']} attempt{run['run_attempt']}，pull_request，13/13成功；未解决线程0；不是main CI |
| 当前host/预算/权限 | 固定门禁PASS；996已知CID停止/PID0/noports/norestart；30权限完整冻结字段false；3新预算空/归属与挂载集精确，27旧预算保留 |
| 备份/失败/pins | 57旧+9新共66备份SHA及历史保留门禁通过，旧失败/已用预算/pins未覆盖 |
| 授权/调度/实际发送 | NOT_RECEIVED / NOT_CREATED / NOT_RUN；DELETE=0；#332 Draft且未合并，#307 OPEN |

{target_table}
十一保留版本的完整identity和保护元组见 `files03/exact-version-crosscheck.json`，与冻结binding一一相等。当前对账不启动Core/native，不再做源库存/IAM网络请求；既有17版本原生验收是已保存准备证据。未来固定窗口内仍须重新跑批准的完整入口，完成fresh Core/native/17库存/保护/IAM/备份/拓扑/预算核验，自动关闭后先Host.check，并逐次生成≤1分钟fresh host proof。没有新授权或任一前置不通过，发送门禁不放行。

每份max1、全局max6，UNKNOWN不重发；逐份send→disable-send→选定Outbox→Asynq只读恢复→Core absence→下一份。H0/H1零发送，H2/M1有限顺序不变。cleanup=0、hold-release=0、bypass=0；失败或过窗立即固定seal，不延长、平移、重开、补发、退款或释放旧pins。

只读核对脚本01/02的解析/schema假设错误及失败已保留，03使用冻结的完整operator/root schema成功；没有修改候选、运行门禁、源码、binary、HEAD或旧事实。新证据仅追加，不提交或推进已锁HEAD。
'''
    with (OUT/'report.md').open('xb') as stream:stream.write(report.encode())
    members={p.relative_to(ROOT).as_posix():{'sha256':sha(p),'bytes':p.stat().st_size} for p in sorted(OUT.rglob('*')) if p.is_file()}
    save(OUT/'evidence-index.json',dict(status='C21_V31_READONLY_CONFIRMATION_CROSSCHECK_INDEX',
        execution=result['execution'],format='files-map-v1',member_root='.',count=len(members),files=members,
        execution_authority=False,native_delete=0))
    print(json.dumps({k:result[k] for k in ('status','at','final_head','ci_run','rule_id','targets','survivors','known_owners_closed','authorization','native_delete')}))


if __name__=='__main__':main()
