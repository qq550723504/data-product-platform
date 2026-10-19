# #298/#299：版本绑定与定向回归交接

2026-10-16，Asia/Singapore。文件名沿用 2026-10-15 的 ownership 登记；本节追加 #318 合并后的定向阶段。当前结论为 **PASS_FOR_DIRECTED_SCOPE**，不是重新运行整轮 B/C，也不替代非开发者人工解释。

## Ownership 与来源

本线程 01a139ad-5931-7822-ad2b-e854c0d449a9 是本交接文档/新证据目录的唯一 writer。隔离分支 codex/issue299-pr318-handoff，工作区为 C:/Users/Henry/.codex/worktrees/issue299-pr318-regression/data-product-platform。生产业务源码没有修改。#307 的活跃 writer、remaining-preparation 工作区及后续生命周期范围保持原 ownership。

权威业务条件仍为 #298/#299、AGENTS.md 以及已有 [C1](issue299-c1.md)、[C2](issue299-c2.md)、[C3](issue299-c3.md)、[独立业务实跑](issue299-independent-20261014.md)。本文件只提供阶段索引、源码差异和追加回归，不建立平行业务规则或重新扩大全生命周期蓝图。

既有 [2026-10-15 ownership checkpoint](https://github.com/qq550723504/data-product-platform/issues/299#issuecomment-6172916443) 原为离线交接。#318 后共享业务代码发生变化，本次按接续任务增加受影响路径的定向运行；[2026-10-16 checkpoint](https://github.com/qq550723504/data-product-platform/issues/299#issuecomment-6209347475) 先明确了新范围。旧报告、manifest、公开 artifact、私有配置及备份不覆盖。

## 阶段索引：原运行、变化、追加回归分开

| 阶段 | 精确身份与证明 |
| --- | --- |
| #298 B 技术收口 | #308 merge 3a59b591d25e7d07fc38956a7ae729d74967215b；其 B1/B2/B3/PGI 与原生 metadata 实跑范围见 Issue 原记录。人工解释继续 NOT_RUN。 |
| #299 C1/C2/C3 | #310 / 1892cd76c4ee578896ddfb3c6f4a7252bfacc51b；#313 / c16720abaf986367bade0e91618133e08785e696；#316 / 79d175d0c8ff7579b0466f44e9103672ac7588da。六项有限技术 PASS 各绑定其原运行，人工条件不勾选。 |
| 2026-10-14 独立业务实跑 | main 074a33bace7edbaf481759aac3a84708363ffd02，run 47203cb1-2b93-4946-8f09-72e29022ea4c。真实 PostgreSQL/Hop/OM、AI Edge、C1/C2、成本、历史及新库恢复均保持该原始身份。 |
| 启动器修复 #319 | reviewed head cf0889346a55184d660a89ce40a937d9fe52584e → merge 196ecda3282d009647afbd79021cf3691bc2fe51。唯一 tag/实际 image ID、OCI 原身份恢复、隔离回归修复，不是重新运行整轮 C1/C2。 |
| #320 / #321 | reviewed head 49322c3e2e83821f52b2c6ca87dc55ddbf7147cd → merge ac8bd7ad316a3977d791bb5f093a1018b9e40fa5。ownership/retain、分项回执、B2 source-binding；#320 已 CLOSED/completed。 |
| 运行器 / CI #322 | reviewed head 19ceaea31713cacbc8edace26542c1ff55a4ab62 → merge d7c9ccdfe4fba066b512fe4adbcd1df281a2d0d9，reviewed/merge tree 相同；[push/main CI38687368151](https://github.com/qq550723504/data-product-platform/actions/runs/38687368151) exact SHA、13/13 SUCCESS。14 项 helper ownership/归档/回收及 CI 变更单列于追加审计，本轮没有重新运行这些 helper 或清理路径。 |
| #318 当前影响 | reviewed head 22ee8e3ed55c38f1e6b7d464081dc068a8628da0 → squash merge b823eb33f8f3f923b619feda06e0ecc53514dd23。该 merge 与 reviewed tree 相同；[push/main CI38842607982](https://github.com/qq550723504/data-product-platform/actions/runs/38842607982) exact SHA、13/13 SUCCESS。 |
| 本次定向运行 | 源码固定为 b823eb33f8f3f923b619feda06e0ecc53514dd23，run 3bffbb42-db97-42a0-a36f-ea2271b3b581。24 个顶层 race tests / 15 个子场景，41 个只读 owner cases，7 个真实 MinIO 检查；0 FAIL / 0 SKIP（下文单列启动/导出 harness FAIL）。 |
| 本交接候选 | 只新增本文和 evidence 索引/回执；候选 head、PR CI 及 Draft 状态在 PR checkpoint 单独绑定，不把当前 main CI 当作本交接 PR CI。 |

旧文档内的 Draft/待评审文字是其原阶段。#319/#321/#318 现已 MERGED；GitHub 关系在 [github-checkpoints.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/github-checkpoints.json) 重新核对，未反向编辑旧阶段。

## #318 影响与定向覆盖

| 边界 | 变化与本次验证 |
| --- | --- |
| Fixture reference owner | Dataset/Entity/Workflow/Quality/Certification/Delivery 新增 owner，8 类 typed reference。当前安装点限 lifecycle-fixture-reader / controlled tests，普通 API/Worker 未安装这些 fixture owner。41 个精确 source/workspace 正例、foreign workspace、错误版本/主体、未知 kind 及 input membership 负例全部 PASS；使用真实 PostgreSQL READ ONLY transaction，8 组引用历史前后 SHA256 相同。 |
| 迁移 67/68 与 admission fence | 普通加工、匹配、质量、认证和交付写入在当前完整迁移下通过；7 个 reference admission 测试覆盖 owner release 保留 FK 历史/active pin、权限和跨租户拒绝、first-copy/source insertion 双向串行化、foreign-schema/control-column 和失效 guard UNKNOWN。全部为新库 SQL/受控 Adapter；没有 provider DELETE。 |
| 加工与 immutable dependencies | Frozen input/retry/traceability、Ready output crash recovery、实体匹配及现有 Native worker 用例通过。额外 Native worker 用例是为了实际生成 EXECUTION_MAPPING，并检查映射/输出和不可变历史；没有无差别重跑整轮 POC。 |
| 质量、认证与交付 | Native Quality/Compliance、实际 retry 成本、Certification/Disposition evidence、共享 delivery fence 均通过。DirectData 服务层验证 committed ISSUED 对应 PayloadReady=true、同 key 返回 ReplayRequired/PayloadReady=false，以及新 attempt 的受控 fresh-gate BLOCKED。此层为真实 PG + 实际业务引擎/命令 + 下文注明的受控输入。 |
| Routing | life2 → life3，新 lifecycle events。5 个路由/版本/全事件覆盖/profile/queue obligation 用例通过；未运行新的 OM 断连矩阵或整个后台投影业务。 |
| Storage/SDK | 新 checkpoint-read 路径的 custody/零重试受控测试通过；实际编译依赖 minio-go/v7 v7.0.88。另以实际 MinIO 和当前 Store adapter 验证读写、拒绝覆盖、并发唯一成功及跨 bucket 预拒绝。 |
| 未变化的 UI/HTTP/行业规则 | 本次没有重跑浏览器、人工、完整 B1/B2/C1/C2 或 OM Connector 矩阵；原结果仍属于其原源码和环境。 |

[impact.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/impact.json) 的 64 项 changes 仅筛选 apps/platform/ 与 migrations/，不是完整仓库差异。Ready 评审发现这一范围标注遗漏后，保留原 40 份证据与 manifest，追加 [完整区间差异审计](evidence/issue298-299-handoff-20261015/ready-impact-audit-20261016/impact-range-audit.json)：ac8bd7ad… → b823eb33… 共 1710 条路径，分为 64 项业务源码/迁移、66 项运维/测试源码、20 项文档和 1560 项既有运行回执；[完整清单](evidence/issue298-299-handoff-20261015/ready-impact-audit-20261016/range-diff.jsonl) 逐项绑定 status、前后 Git blob 与 SHA256。入清单只证明差异身份，不表示这些路径全部在本轮运行。

#322 的 14 项文件在追加审计中分别绑定 merge 时与目标 b823 的源码。其四个 lifecycle wrapper（finish-prepare、run-copy-recovery-step、run-plane-step、run-reader-step）接入临时 helper 归档/回收，本轮驱动没有调用它们或 temporary-container helper。本轮实际复用 retained_runtime → smoke/retained-storage.ps1 → durable-storage.ps1；这四份输入在该区间均未变化，追加审计记录了精确 hash。#322 原 CI 与 #318 其他运维/测试/回执保持各自证明范围，本轮不重跑清理、接管环境或替 #307 追加业务验收。

[source-history-audit.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/source-history-audit.json) 核对旧 759 份源码：733 UNCHANGED、26 CHANGED、0 MISSING，旧 hash 全匹配其原基线。不能再沿用“业务源码无变化”的 2026-10-15 区间结论。

## 本次环境、二进制与证明粒度

- 新 PostgreSQL 16.15，容器 dpp-bc318-3bffbb42db97-pg，loopback port 54270；唯一新命名卷 dpp-bc318-3bffbb42db97-pgdata。全部 scratch databases 保留，最终创建了 16 份数据库 dump；本次没有另做恢复验收。
- 新 MinIO 成功夹具 dpp-bc318-3bffbb42db97-minio02，loopback port 57792；唯一新命名卷 dpp-bc318-3bffbb42db97-objects02。复用实际 image ID 4b343185d3ce9148dbcb2ca720379faa793a140d54d6998b1cfe26f514fbb7fa，没有 build/tag 或接管原卷。实际 binary 自报 DEVELOPMENT.GOGET、commit 9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a、Go1.26.5 linux/amd64；不据此声称生产 provider admission。
- 两项持久服务都复用现有 DurableStorage / Assert-RetainedServiceStorage，在 created 和 running 状态验证 run label、角色唯一 named volume、数据/WAL mount 无覆盖。失败夹具也保留。
- Go1.26.5 windows/amd64；race test binaries 与 migrate/native storage/owner helper 共 13 个实际二进制 SHA256、build settings、模块版本见 [binary-identities.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/binary-identities.json)。
- [current-source-bindings.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/current-source-bindings.json) 对 802 个原来源和增量源码分别保存 Git blob OID、Git LF SHA256、实际测试工作区字节 SHA256。执行 helper source snapshots 与二进制绑定另列，不把临时测试驱动冒充提交内生产源码。
- 现有 lifecycle test helper 会 DROP 自己的新数据库。为遵守保留要求，仅通过标准 Go overlay 将那一个 cleanup hook 替换为 retained-name 日志；全部业务断言和生产源码未改。原/覆盖源码及移除 hook 的 hash 已记录；所有新测试库保留，不声称运行了未覆盖的测试配置。

7 项真实 MinIO 检查：create-only PUT、UTF-8/负数原值 GET、重复 PUT 拒绝覆盖且旧 bytes 不变、并发两个 PUT 仅一成功且 winner bytes 稳定、错误 bucket URI 在发请求前拒绝、零 DELETE。只记录 method/path/status 的 loopback proxy 捕获 12 次 HTTP。该新 bucket 的 Adapter contract 不等于真实用户 CurrentDeliveryGate→Provider 完整联合路径，也未证明 gateway IAM、版本控制/WORM 或生产权限；DirectData fresh gate 的 PG 回归与真实 SDK adapter 验证分列。

认证回归使用现有 staticEvidenceResolver；DirectData 两个选中测试仅调用 DirectDataService，使用现有 directIntegrationGate 与 PostgreSQL repositories，没有接入 Reader 或 HTTP payload 输出。该层本次 PASS 仅覆盖受控输入下的事务/fence、immutable decision evidence、committed ISSUED/PayloadReady、同 key ReplayRequired/PayloadReady=false，以及 replacement attempt 的 fresh-evaluation 协议；实际 HTTP 响应首字节行为、完整 SQL CurrentEntitlementGate 与真实 payload 联合交付本轮 NOT_RUN。其业务规则未被 #318 修改，原业务实跑与 main CI 继续按各自身份引用，不从本轮受控输入推导新的完整权利/交付 PASS。

## 回执与原失败保留

- [targeted-tests.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/targeted-tests.json)：23 顶层 / 15 子场景；[额外 Native worker](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/extra-native-worker.json)：1 顶层。所有测试 0 FAIL、0 SKIP，完整结构化输出另附。
- [module-owner-checks.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/module-owner-checks.json)：41 cases / 8 classes / 6 modules，前后引用历史 hash 相同。
- [native-storage-report.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/native-storage-report.json)：真实 Provider 7 checks / 12 HTTP / 0 DELETE；不是受控响应替身。
- [backup-summary.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/backup-summary.json)：本轮新库备份 CREATED、恢复 NOT_RUN；原库恢复仍 DEFERRED。
- [harness-failures.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/harness-failures.json)：初次新 MinIO 因驱动错误入口 /minio 启动 FAIL，实际既有入口 /usr/local/bin/minio；原容器/卷原样保留，另建 minio02。Native 检查已完成后，记录器默认 GBK 读取 UTF-8 helper source 导出 FAIL；原始 JSON 保留，只修复 UTF-8 导出，没有重发 Provider。两项都是本轮驱动/记录失败，未伪装成成功，也未归为产品缺陷。
- C3 18、旧独立验收 92、trial images 30，合计 140 份原 artifact Git 字节哈希全部匹配，旧报告与 manifest 未修改。新目录有独立 LF 属性和 [artifact-hashes.json](evidence/issue298-299-handoff-20261015/pr318-targeted-20261016/artifact-hashes.json)，避免 Windows checkout 编码/换行改变证据。
- 新目录已对本轮已知实际凭据值扫描，0 命中。私有配置、环境文件、完整 dump/globals、实际二进制与完整技术 trace 只留在受保护本地目录；公开内容不包含这些 credential values。

## 剩余必须完成项和限制

#298/#299 继续 OPEN。两项的非开发者操作/解释仍 NOT_RUN / 暂缓；AI 浏览器、自动化或本次定向回归都不代替人工。原 dpp-pgi-b3-core-bc776a791192 继续 FAIL / NOT_RESTORED，恢复暂缓；本轮新库及 dump 不能证明旧历史恢复。

初始定向运行无产品 blocker、无必要共享源码修复，交接以 Draft 文档/证据增量提交；后续 Ready、独立评审及合并按新的用户授权在 PR checkpoint 分别记录。所有原保留环境、容器、共享标签、卷、对象与备份保留；旧来源未重新授权，未执行 cleanup 或 provider DELETE。#307 的真实 processing/DirectData-versus-DELETE、in-flight late hold、三 copy mixed outcomes 仍属其原 owner / open Issue；本轮未推进或代验这些范围。
