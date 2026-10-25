"""Compose the single authorized date's heartbeat using current frozen bindings."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import tomllib
ROOT=Path.cwd();BASE=ROOT/'docs/acceptance/evidence/issue307-d1';OUT=Path(__file__).resolve().parent
PACKET=BASE/'remaining-candidate19';PRIVATE=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19')
def load(path):return json.loads(path.read_bytes())
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def need(ok,message):
    if not ok:raise ValueError(message)
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
c=load(PACKET/'candidate.json');a=load(OUT/'human-authorization.json');s=load(OUT/'schedule-human-authority.json');sf=load(PACKET/'source-freeze.json')
source_report=BASE/'candidate19-source-review01/independent-final-review01.json';packet_report=BASE/'candidate19-review01/independent-final-review01.json'
for path,status in [(source_report,'PASS_BOUNDED_C19_SOURCE_PREPARATION_REVIEW'),(packet_report,'PASS_BOUNDED_C19_FROZEN_NONDELETE_PREPARATION_REVIEW')]:
    value=load(path)
    need(value['status']==status and value['remaining_blockers']==[] and value['execution']==c['Execution'],'Frozen independent review differs')
need(c['Execution']==a['execution']==s['execution'] and s['answer_verbatim']=='安排固定窗口续跑（推荐）','Authority scope differs')
need(dt.datetime.now(dt.timezone.utc)<dt.datetime(2026,10,22,6,0,tzinfo=dt.timezone.utc),'Window already opened; do not schedule a later date')
previous=[]
for path in Path(r'C:\Users\Henry\.codex\automations').glob('*/automation.toml'):
    value=tomllib.loads(path.read_text(encoding='utf-8'))
    if 'candidate19/v29' in value.get('prompt','') or c['Execution'] in value.get('prompt',''):
        previous.append({'id':value.get('id',path.parent.name),'status':value.get('status')})
need(not previous,'Matching automation exists; inspect/update the same id, never duplicate')
prompt=f"""续跑本聊天 #307 D1 已获本聊天直接人类删除授权及单独调度授权的 candidate19/v29，execution {c['Execution']}。唯一日期2026-10-22，新加坡14:00–14:40，即UTC06:00≤now<06:40。只在此窗口且全部新鲜前置通过后，按冻结顺序P2→T1→T2→H2A→M1A→M1B删除六份exact目标，每份最多一次、全局最多六次，UNKNOWN不重发。P1永久排除；11份survivor、其他环境、全部备份、旧pins、新旧预算、失败/UNKNOWN/provider attempts/成本事实保留；cleanup=0、hold-release=0、bypass=0。#332保持OPEN/Draft，不Ready、不合并；#307保持OPEN。不延长、平移、重开或补发。早醒只能离线/关闭只读核验，发送关闭；晚醒或超窗保持/完成封存并报告实际未执行。完成、失败、过期或已有本execution terminal时暂停本同一heartbeat 307-d1-candidate19-v29，不创建重复调度或再执行。无实质变化时安静，只报告阻断、失败、完成或需人类处理的事项。

唯一工作目录 {ROOT}。不能用9400、d1-c18或其他旧入口。先读本聊天最新真实人类消息，撤销/收窄优先，核实际UTC及终态。私有控制目录 {PRIVATE}，实际授权、运行事实及终态在authorized-execution01；公开执行回执在docs/acceptance/evidence/issue307-d1/delete-confirmed-v29-execution01。若已有terminal仅只读报告并暂停自身；若已有启动/预算消耗事实而无终态，先核原owner及不可变Core事实，不能重启或重发，无法证明安全继续则固定seal并保留UNKNOWN。未运行项不补发到其他窗口。

直接删除授权来自真实人类消息{a['human_message_id']}，app消息{a['human_app_message_id']}，时间{a['human_message_timestamp']}，turn {a['human_turn_id']}。用户明确按已核对candidate19/v29完整方案及全部冻结绑定授权，仅此execution/固定日期窗口/六目标max1/max6/UNKNOWN不重发/全部保留限制/失败过窗即封存。实际原文含末尾LF，UTF8 {a['user_message_utf8_bytes']} bytes，SHA256 {a['user_message_sha256']}，在私有authorized-execution01/user-message-verbatim.txt。user_retyped_template=false，实际人类原文与批准机器模板分别留证，不能trim或伪称人类重新键入127行模板。公开human-authorization.json SHA256 {sha(OUT/'human-authorization.json')}；私有authorization-envelope.txt {a['authorization_envelope_sha256']}，approved-host-bindings.txt {a['additional_host_bindings_sha256']}，approved-reference-manifest.json {a['approved_reference_manifest_sha256']}。924项approved_reference_artifacts当前实际字节须全部匹配。session中的完整展示额外含memory citation，read_thread展示文本省去该渲染元数据，业务正文和模板完全一致；两个原件已分别保留，不混称原字节相等。旧授权绝不复用。

单独调度授权来自真实人类消息{s['human_message_id']}，app消息{s['human_app_message_id']}，时间{s['human_message_timestamp']}，turn {s['human_turn_id']}。实际回答“安排固定窗口续跑（推荐）”，问题明确同一execution、2026-10-22新加坡14:00的一次续跑、14:00–14:40全部新鲜前置以及完成/失败/过期停止。实际wrapper UTF8 {s['actual_wrapper_utf8_bytes']} bytes，SHA256 {s['actual_wrapper_sha256']}。公开schedule-human-authority.json SHA256 {sha(OUT/'schedule-human-authority.json')}，schedule-authority-index01.json SHA256 {sha(OUT/'schedule-authority-index01.json')}。这只授权本次调度，不扩大删除。旧07/10/11/12/15/16/17 heartbeat保持PAUSED，旧18已删除，不恢复任何旧调度。原准备status/冻结手册/旧crosscheck的NOT_RECEIVED/NOT_CREATED、授权checkpoint当时schedule NOT_CREATED是不可变历史快照，不覆盖，也不据此否认后来单独授权或调度。

必须完整阅读docs/acceptance/issue307-d1-candidate19-confirmation.md（SHA256 {sha(ROOT/'docs/acceptance/issue307-d1-candidate19-confirmation.md')}）与issue307-d1-v29-window-runbook.md（SHA256 {sha(ROOT/'docs/acceptance/issue307-d1-v29-window-runbook.md')}）。exact namespace/key/version/copy/DV/plan/hash、11survivor及保护状态，以批准完整机器模板和remaining-candidate19 packet为唯一范围和顺序，不从相似key、旧模板或投影猜目标。六目标仅business-races project dpp-lifecycle307-808a6960的P2/T1/T2、late-hold project dpp-lifecycle307-754ea8f0的H2/A、mixed project dpp-lifecycle307-dfda43b4的M1A/M1B。18历史copy与17现存版本严格区分：6目标+11survivor；P1仅保留真实NoSuchVersion404历史事实，永不重跑。

冻结packet：candidate SHA256 {sha(PACKET/'candidate.json')}；bindings {sha(PACKET/'bindings.json')}；template {sha(PACKET/'confirmation-template.txt')}；source-freeze {sha(PACKET/'source-freeze.json')}；profile-windows {sha(PACKET/'profile-windows.json')}；cross-field-windows {sha(PACKET/'cross-field-windows.json')}；target digest {c['TargetDigest']}。运行source {c['Source']}，{len(sf['files'])}选定源码成员各自raw/LF SHA，私有approved-runtime-source.zip SHA256 {sf['archive_sha256']}、{sf['archive_bytes']} bytes。Git/LF/ZIP/raw分别核对，不冒称全部raw=Git。12原build成员包括reader {c['ReaderBinarySHA256']}、sender/wire/recovery {c['SenderBinarySHA256']}、root owner {c['OwnerBinarySHA256']}、Windows file-only checker {sha(PRIVATE/'readonly-check.exe')}及8原空stdout/stderr日志。不得修改source/candidate/binary/config，不重跑allocate/prepare/materialize/initialize-budgets/run-non-delete/freeze或重建资源规避漂移。窗口内仅按手册给选中固定owner批准模板原字节，不是重做准备阶段物化。

36成员宿主索引candidate19-planning01/candidate19-host-bindings01.json SHA256 {sha(BASE/'candidate19-planning01/candidate19-host-bindings01.json')}；原893项approval-reference-manifest01.json SHA256 {sha(BASE/'candidate19-planning01/approval-reference-manifest01.json')}；实际新授权追加原manifest本身、发布和逐项核对证据合计924项。唯一维护入口examples/lifecycle-d1/window_six_support.py SHA256 {sha(ROOT/'examples/lifecycle-d1/window_six_support.py')}；复用review06封存依赖SHA256 {sha(BASE/'remaining-window-review06/composition/window_support.py')}。packet Git anchor749257f15e13ee7ab64b4b828857e2e88a054fbc仅供seal从同一SHA验证已知完整CID并继续关闭，不授予发送；当前包损坏也不能阻止已知owner关闭。私有host-anchor.json SHA256 {sha(PRIVATE/'host-anchor.json')}。禁止旧代入口。

源码独立报告candidate19-source-review01/independent-final-review01.json SHA256 {sha(source_report)}，必须PASS_BOUNDED_C19_SOURCE_PREPARATION_REVIEW且remaining_blockers=[]；冻结独立报告candidate19-review01/independent-final-review01.json SHA256 {sha(packet_report)}，必须PASS_BOUNDED_C19_FROZEN_NONDELETE_PREPARATION_REVIEW且remaining_blockers=[]，全部索引实际字节匹配。准备评审不是人类删除授权，也不声称后来新增授权/调度已另经独立复评。v28 P2 HTTP403 CONFIRMED DENIED及已用预算/成本/attempt原事实、v26/v27和全部旧失败/UNKNOWN/pins均保留；实际新鲜通过不得回写旧失败。

实际HEAD及PR head必须仍{a['final_approved_head']}，#332 OPEN/Draft/unmerged、#307 OPEN。执行前实时重核该精确HEAD全部必要PR CI成功、评审线程和Issue状态。当前run39710200629，event=pull_request、attempt1、13/13SUCCESS、未解决线程0；不是main CI，不能拿历史HEAD绿色替代。新增未跟踪授权/调度/核对/执行回执与矩阵不是运行源码变化；任何tracked源码/冻结字节/HEAD漂移即停。Git仅子进程core.longpaths=true或GIT_CONFIG_COUNT/KEY_n/VALUE_n，不改全局/local配置。

授权后03:01Z固定Host.check PASS_PREWINDOW_CLOSED：实际新授权闭合，6case/6recovery精确、30permissionfalse、86本代owners关闭、3新预算为空、9新+48旧backupSHA通过。此前02:47Z820已知CID全部stopped/PID0/noports/norestart、21旧预算SHA/完整挂载集保持，v28 P2旧已用预算2文件保留；57备份包括54完整+3失败partial。以上不代替窗口内fresh Core/native/IAM/17版本证明。窗口前禁止enable/Source/case/wire/sender或签发送proof，worker不得提前物化批准文本。

开窗首先使用冻结完整入口examples/lifecycle-d1/verify-six-native.py SHA256 {sha(ROOT/'examples/lifecycle-d1/verify-six-native.py')}；不能临时拼预检脚本。精确CLI参数：--candidate-sha256 {sha(PACKET/'candidate.json')} --bindings-sha256 {sha(PACKET/'bindings.json')} --checker-sha256 {sha(PRIVATE/'readonly-check.exe')} --evidence-audit-plan docs/acceptance/evidence/issue307-d1/delete-confirmed-v29-execution01/scheduled-evidence-audit-plan01.json --evidence-audit-plan-sha256 {sha(OUT/'scheduled-evidence-audit-plan01.json')} --output 本工作目录内尚不存在的新绝对evidence子目录。这个追加phase plan已通过相同完整入口--audit-only，10显式索引/5473引用（原7索引4539+实际批准924+真实删除授权7+单独调度3）。各indexSHA独立钉住，严格区分files-map-v1与members-list-v1，不递归猜格式。原准备plan/授权index不覆盖；后来的scheduled-prompt/automation.toml/调度读回另存，不加入此前plan制造自引用，也不要求当前toml等于创建前快照。离线审计PASS不等于native预检。

完整入口先核六实际case.Batch=本execution、Base.StartedAt=2026-10-22T06:00Z、Base.ExpiresAt=CandidateUseUntil=06:40Z，六recovery.ValidUntil=08:40Z；语义早于permission/Docker/Source I/O。remaining_six_window.py SHA256 {sha(ROOT/'examples/lifecycle-d1/remaining_six_window.py')}，remaining_window.py {sha(ROOT/'examples/lifecycle-d1/remaining_window.py')}。按角色固定descriptor核真实CID/image/command/完整mount/netns/noports/no-socket/no-restart、私有文件白名单；worker UID10001、固定tmpfs/cap-drop/no-new-privileges，不挂root/host私钥，PG/native/Redis/Caddy/initializer按其service角色，不误套worker约束。30false、3internal restore networks、source/restore不别名、3新budget归本execution/完整挂载CID集合精确/首次发送前为空。Docker raw wrapper只bytes，文本严格UTF8；类型错误即停，不补丁续跑、不删除或重建预算。

同一入口新鲜核3源Core REPEATABLE READ READ ONLY全关系/roles/ACL前后不变；17exact bytes/SHA/full namespace inventory/hold/retention、P1真实NoSuchVersion404；41 known IAM全disabled，其中40sole-policy/no-groups与唯一c13 partial i4e524c04808a6960严格NONE/no-groups、未分配policy单独官方Equals，9 absent fresh NoSuchUser。新代delete action pair必须完全同一exact version条件，旧代Version-only冻结策略不改；官方ParseConfig/Equals核完整policy，不能用raw SHA或2xx-only。重核57备份SHA、分离restoreCID和旧环境/预算/pins/失败保留。每次technical GET/HEAD/admin保存start/observation/真实HTTP/provider attempt/成本，amount未知NOT_ASSESSED；NoSuchObjectLockConfiguration400不表示对象缺失。任何非预期失败停止，不修改门禁续跑。完整CLI自动finally-close Source/技术helper后、任何重启Core/native之前，先固定window_six_support.py check通过本次实际授权及全部资源stopped，不能先启动再做要求stopped的check。

随后只在固定窗内启动绑定三Core/native并复核，六slot和恢复期间Core保持可读，下一slot proof直接读实际Core immutable absence，不读host进度projection。无Renew、新Rights、Processing重跑、DirectData再传输或P1重跑；五business policy heads只读核checkpoint {c['PolicyCheckpointSHA256']}。root O_EXCL result/status-changes存在时先archive-root保留原字节/SHA/journal，归档失败禁止启用。窗口内才给选中固定owner批准机器template原字节，permission仅变Enabled/真实ConfirmedAt，其他绑定/窗口/max1/max6不变，未选ownerfalse。安装冻结active gateway并核实际mountSHA，固定enable-read只开本代限定inspector。root bootstrap与worker物理分离，host私钥不挂worker，wire独立私钥仅wire。拓扑签名器attest-remaining-executable.py SHA256 {sha(ROOT/'examples/lifecycle-d1/attest-remaining-executable.py')}，host public {c['PublicKey']}，wire public {c['WirePublicKey']}。

每sender/wire/recovery前各自≤1分钟fresh实际host topology，wire运行中逐target刷新proof，context≤3分钟，SDK无重试，持久target/wire预算必须先于Core claim。每slot严格send→disable-send→仅选定Outbox EventID经既有Asynq只读recover→Core提交immutable absence→下一slot；前序发送未关或absence未提交不能消费下一slot预算，不使用旧四send批次，不抢窗补发。P2/T1/T2只复用已完成Processing/DirectData与可信owner/handles/pin/ref闭合，新增本execution fixture basis/批准/dispatch，不复活旧已撤销批准或attempt。
H0/H1零send。case-late成功exit0/PID0后revoke-case仅撤销Base.Enabled并私有保留原profile，H1 owner永不重开；所有late operator/rootfalse、enablers实际停止、固定disable-all、≤45秒fresh check-disabled、gateway实际停止sealed后h1-proof签一次可信closure，随后≤1分钟固定close-H1在sealed gateway追加NOT_SENT。不得手填布尔值、用旧读回或自动重签。后续仅固定窗内受控active gateway和限定inspector，H1 case保持撤销。
H2仅A一次，真实上游204后才witness/late Core holds A+B/revoke/响应丢失，形成CONFIRMED UNKNOWN；B永不dispatch。预期UNKNOWN仅沿固定只读链处理，绝不重发。
M1A一次并提交absence后才M1B一次，形成CONFIRMED UNKNOWN+C-only保护阻断mixed checkpoint；原checkpoint逐字节复制到recover-M1B和case-mixed-c并核SHA之前禁止B lookup或全namespace query；之后仅B只读lookup，C永不删除。
六恢复仅冻结recover-*、选定Outbox→既有Asynq及独立readonly profile，最晚2026-10-22T08:40Z（新加坡16:40），不能重新enable身份或延长发送窗口。IAM关闭不能lookup时保留UNKNOWN/NOT_RUN，不额外enable。06:40Z起禁止新增wire/sender/enable，不抢窗使用剩余预算。

任何非预期失败立即停止新增send，用唯一window_six_support.py seal：关全部30permission/case/recovery，停全部sender/wire/enable owners，各组只运行固定disable-all，再停/封gateway，继续所有绑定source/Core/native、恢复Redis及原business Redis至PID0。一项失败继续其他组；inspect失败仍尝试已知完整CID。root归档失败关闭时仍尝试disable-all；无fresh identity读回、enabler未闭合、归档不完整或未知owner均标UNCONFIRMED，不报整轮PASS。seal不启动Source/enable/sender/wire；不cleanup/hold-release/bypass/release/退款，不覆盖旧失败/UNKNOWN/attempt/cost/budget/pins/backup，不resend。终态和原字节SHA索引追加同一private authorized-execution01与新public执行子目录，实际未跑NOT_RUN，完整D1不提前接受。成功、失败、过期后暂停本同一heartbeat，不建新调度；窗口后只读期限不是发送或重新启用授权。"""
args={'kind':'heartbeat','mode':'create','name':'#307 D1 candidate19/v29 固定窗口续跑','prompt':prompt,'rrule':'FREQ=DAILY;COUNT=1;BYHOUR=14;BYMINUTE=0;BYSECOND=0','status':'ACTIVE','destination':'thread','targetThreadId':a['thread_id']}
write(OUT/'scheduled-prompt01.txt',prompt.encode())
write(OUT/'schedule-create-arguments01.json',(json.dumps(args,ensure_ascii=False,indent=2)+'\n').encode())
print(json.dumps({'prompt_bytes':len(prompt.encode()),'prompt_sha256':sha(OUT/'scheduled-prompt01.txt'),'args_sha256':sha(OUT/'schedule-create-arguments01.json'),'matching_existing_automations':previous,'native_delete':0}))

