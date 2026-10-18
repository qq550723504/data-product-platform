# #307 D1 R1 有限修复契约

目标：新 execution 的六项 Renew 不再继承旧模板期限。`remaining_scope.go` 的编译常量是当前 execution、profile、START、END 的唯一来源；生成器从 END 派生 CandidateUseUntil 与恢复截止 END+2h。

必须满足：七个 case 的 Batch、Base 起止、CandidateUseUntil 与当前候选一致，六个 expected policy heads 绑定保持；七个 recovery 的截止与 END+2h 一致。公开语义投影绑定实际私有配置 SHA；离线校验检查字段关系，完整预检在 permission 读取、Docker inspect/start 之前重读实际配置。重算 SHA 不能豁免语义错误。runtime 的严格截止门禁保持。

验收范围：生成器旧模板正例；跨字段反例在生成器/完整 CLI/离线入口拒绝；Go 无数据库反例；独立 PostgreSQL 备份副本上的六项非删除 Renew；新的完整原生只读库存/IAM/Core 核验；独立评审。复用现有 pg_restore、reader Command、官方 policy 语义检查及标准库 datetime/json，不新增工作流或策略基础设施。

历史：candidate10/v20 的失败、旧授权、旧配置与原始证据不修改。旧检查器仅在其冻结源码中审计，不能成为新运行授权。技术 GET/HEAD verifier 的临时时限与旧库存 baseline 不冒充新 case 配置。

新候选为 candidate11/v21，execution `602acafe-065a-4385-b739-5776408f5448`，提议固定窗口 2026-10-18 新加坡 22:00–22:40（UTC14:00–14:40），只读恢复最晚 UTC16:40。该窗口由人类选择用于准备，尚无删除授权或调度。候选冻结后另行精确确认；失败或过窗立即封存，禁止移窗或重用授权。

阻断仅限本契约必要路径、权限绕过、历史/证据丢失或 CI 失败。真实七项 DELETE、H2/M1 实际结果、全量 D1 验收、Ready/合并均不属于本轮；#323保持Draft，#307保持OPEN。
