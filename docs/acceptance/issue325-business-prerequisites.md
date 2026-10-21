# #325 显式业务前提准备（候选交付，不是执行授权）

## Merge contract

Goal：在已经激活的同一合成 trial 中，通过只读 `business-plan` 和单独的
`prepare-business` 准备 B 的业务前提，使已有 CURATED 可以继续独立质量、合规、
认证与交付操作。准备不等于这些操作已经执行或通过。

Authoritative sources：本文件拥有装配/恢复边界；Core rights、contract、certification
Commands 继续拥有业务语义；`AGENTS.md` §1A/3/7/10/17、#325、现有 activation contract
拥有复用、不可变历史、交付及原环境准入规则。README 只提供入口用法。

写域仅限 `cmd/trial-business`、`deploy/trial`、本文件和对应 CI path filter。
不改 Core、共享 storage/worker/router、迁移或 #307 的写域。运行身份仍是已激活
run 的 workspace、Actor 与 `trial-operator-<runId>`。不得以数据 owner 推断现实权利。

Reuse check：已有 Rights Create/Submit/Approve/Activate、declaration Create/Verify、
BindAuthorizationProvenance、Contract Create/Publish、Profile Create Commands 可复用。
复用 PostgreSQL session advisory lock、command_idempotency 与同事务 AuditEvent，
而非新建 workflow/queue。Ansible/Temporal 不能取代这些 Core 事务事实；再引入它们
会形成平行装配/状态源。本次无新依赖、IAM、数据库表或通用基础设施。

## 有限准备范围

- plan 绑定 schema71、原五份 READY 版本及其完整 SQL 身份、原已接纳批次和成功
  execution、实际 lineage closure、三份 RAW 的 source resources、同一 activation。
  只读 SQL 事务；不读对象、不构建、不创建目录/锁/诊断文件。
- private spec 明确 consumer、固定 purpose、有限 validity、合成 grantor 与核验理由。
  权限只有三来源的 READ、每个来源自己的 ALL_RESOURCE scope、单 consumer 和
  purpose；既有 Core STS policy 完全不变。衍生输入由 Core 后续 Effective Rights 与
  CurrentDeliveryGate 遍历，不能只授予 CURATED 来绕过 lineage。
- prepare 只创建并激活一条 scoped Authorization、三条有限声明/核验/绑定、一个
  单 consumer/purpose 的试用合同版本和 frozen CertificationProfile。所有业务写入
  经既有 Commands，保留它们的 Audit/Outbox/Evidence/真实 activity Cost。
- 不计算质量/合规、不创建 certification/effective-rights/evidence snapshot，不创建
  DatasetVersion、不重新审核/加工，不运行 remediation、下载或撤权。
- 必要配置显式绑定成功回执后才影响下一次被单独授权的宿主启动：Quality/Compliance
  动作使用原 Actor/workspace；remediation 复用既有 plan；Delivery 仍关闭。交付 token、
  trusted principal→consumer 配置和真实交付/撤权须另行审查授权，不能因准备自动启用。

## 幂等与中断

每个 run 只有一份 Core preparation fingerprint，覆盖 spec、原事实、规则/合同内容。
外层 reviewed plan 另外绑定 controller/tool；UNKNOWN/PASS projection 保存后，改变
这些输入同样拒绝，不自动重置或更新。首次 journal 已写、registry 尚未保存的窗口
还没有调用 helper 或写入 Core；这类 orphan 仅保留观察，不冒充已提交的业务步骤。
prepare 在原 operator lock
内重新生成计划，再获取 Core session advisory lock。首个 Core 写入是既有
command_idempotency 的 fingerprint claim；它不是第二套业务事实。

每个 Core Command 使用稳定、隔离的 trace/activity。成功 Commands 的 AuditEvent
与业务事实在同一事务提交；重放先查询该 action/workspace/Actor/trace 的唯一审计
事实，恢复 exact object ID。没有审计事实才调用 Command。事务未知时保留 claim，
下次沿同一 fingerprint 重读 Core：已提交不重复，未提交才继续。已有别人的同名
对象不会被采纳；唯一键/Command 拒绝。不得覆盖初次失败回执。已完成重放不再调用
Commands，不重新激活已撤销授权；准备回执不代表当前交付资格。

## Blockers / non-goals

Blockers：计划产生写副作用；身份/lineage/workspace 漂移未拒绝；同 key 输入冲突、
中断/并发导致重复业务或成本；绕过 Commands/门禁；secret 泄漏；原历史被修改；
当前必需路径或 CI 失败。

非目标：生产 IAM/法律裁定、自动续期、通用恢复、业务评测/认证 PASS、C1 修复闭环、
OpenMetadata、人工解释或 #325 关闭。旧 quarantine 不是 remediation case；C1 要求
单独授权新的 remediation Execution/输出，不能把旧 execution 伪造成修复。

验证：隔离 PostgreSQL 的真实 Commands、只读计划、重复/冲突/中断恢复/并发、
跨 workspace 和 lineage 拒绝、原版本/历史保留；Python CLI 和配置绑定回归；当前
HEAD 独立评审与 CI。原环境验收、部署、业务执行全部 NOT_RUN。
