# #298 最终技术切片实跑记录

日期：2026-10-11。基线 main `9502bd1caea61dd86e80827142bd1d18269a2d79`；独立 `codex/issue298-closure` worktree。运行期间 main 从 `b69433df` 增加 #306 的四份生命周期文档，已 fast-forward；`apps/`、`migrations/`、`industry-packs/`、`examples/` 内容没有随该更新变化。本记录是合成、loopback 技术验收；人工解释继续 **NOT_RUN**，不关闭 #298，不自动合并。

## 结果与范围

| 验收 | 结果与实际证据 |
|---|---|
| 最终代码 B3 | **PASS**：真实 Edge 在正式 Next 构建服务器完成来源/批次登记、Hop 抽取、三份 RAW 接纳、实体审核、既有加工、Quality、Compliance、证据冻结、Certification 和 Direct Data 下载；[独立 SQL/HTTP oracle](results/2026-10-11/closure/b3-report.json) 32 PASS |
| 值与产物 | RAW 原始 6/10/14 行 hash 不变；加工隔离 M-005 原始 `energy_kwh=-999`，生成新 CURATED。两次真实浏览器下载均为 5 行、640 bytes、SHA256 `3be783926e4860bde75a6e01b561455c0a7086dcb54c6bc54e4f768a8417a9ac` |
| 质量失败不认证 | RAW 实际 Quality FAIL → Certification REJECTED，包含 `QUALITY_GATE_NOT_PASSED`；该 RAW 同时缺其他认证证据，不声称这是“其他前置条件全部有效”的单变量浏览器实验。独立 Quality 阻断由既有 certification Domain tests 覆盖 |
| 当前授权 | 两次 ISSUED；两次撤销后从旧 ALLOWED 页面创建的显式 retry 均 BLOCKED，0 数据载荷。原 ISSUED 和旧认证保留；重新认证后显式 SUPERSEDED 退出旧认证 |
| 治理展示 | 原生 OpenMetadata 2.0.5 描述、Owner、显式 Steward、Classification 与 Glossary 只读显示；energy 未配置 Steward 显示“未指定”。Core owner/sensitivity/revision 独立、未变化 |
| 失联与恢复 | 停止 **同一个**独立 OpenMetadata server → 三来源 STALE、当前治理字段 UNKNOWN、绑定缓存版本 0.1 保留；失败刷新不修改 binding。重启同容器/数据卷 → 同 ID/FQN OBSERVED |
| 非空历史发布保护 | 两份真实 PUBLISHED，每份有 1 个发布成员、5 个 ancestry DatasetVersion、非空 lineage、3 个 Evidence items。逐组完整行/成员 hash 在断连、恢复、投影重试及最终授权撤销后相同；[五阶段报告](results/2026-10-11/closure/history-post-revocation.json) |
| 原生投影恢复 | 第一份通过实际 ProductReleased → Outbox/worker 投影 SUCCEEDED attempt 1。断连期间第二份 Core 发布成功，投影 FAILED attempt 2；恢复后同一事件由既有 worker 自动重试 SUCCEEDED attempt 4。原生 DataProduct ID `1dd27fe8-921b-4fe5-8d76-b14233e9f292`、`domains=[SyntheticPark]` 与两份 projection 相同 |
| 密钥隔离 | 初次开发服务器 HTML 在 RSC fetch 调试记录携带测试 token，判定 **FAIL**、终止该 disposable runtime：[无 secret 的 RED 记录](results/2026-10-11/closure/secret-isolation-red.json)。启动器改为 `NODE_ENV=production` + `build/start` 后，在全新 Core 重跑全部 B3；已知来源/Hop/Core/MinIO/平台/OpenMetadata/gateway 密钥不出现在 HTML、业务事实、Audit 或 Outbox 中 |
| 本地检查 | 完整 `go test ./...`、`go vet ./...`；metadata/Port `-count=1`；web lint/typecheck；正式 build 内 204 command tests 和 Next 构建；Python compile、`git diff --check` 全部 PASS。PR CI、评审、main CI 另行核对，不由本地检查推断 |

运行身份、实施源文件 hash、引擎与产物见 [identity.json](results/2026-10-11/closure/identity.json)。原生 provider 的 `domain` 旧请求真实 400，`domains` 请求真实 201，分别保存为 [RED](results/2026-10-11/closure/native-contract-red.json) / [GREEN](results/2026-10-11/closure/native-contract-green.json)；GREEN 的 probe 不是 Core 发布投影证据，实际 projection proof 在五阶段报告。

## 非空保护 oracle

`closure_verify.py` 只读真实 PostgreSQL/Core/native API。每阶段保存 release header/membership、ProductVersion/assets、完整 ancestry versions/lineage、Quality/Compliance、RightsSnapshot 及其 authorization/declaration/binding membership、EvidenceSnapshot/root/items/正文、publish Audit 的完整行 hash 与数量；拒绝零发布、空成员、空证据。`governance_projection` 是可变投影，单独记录 status/attempts/event/provider identity，不与冻结发布事实混为一谈。

依次为 [healthy](results/2026-10-11/closure/history-healthy.json) → [outage](results/2026-10-11/closure/history-outage.json) → [projection failed](results/2026-10-11/closure/history-projection-failed.json) → [recovered](results/2026-10-11/closure/history-recovered.json) → [post revocation](results/2026-10-11/closure/history-post-revocation.json)。前三类当前 gate 在目录故障期间保持 ALLOWED；最终撤销时 DatasetVersion/Certification 仍 ALLOWED，Entitlement 为 BLOCKED，全部历史 hash 保留。

## 可复现入口

沿用 `pgi09_openmetadata.py prepare/seed` 与官方 pinned Compose 2.0.5；`seed` 只允许全新合成目录。Steward 使用原生 Type API 的 `entityReferenceList` custom property `dataStewards`，不把 Owner 或团队名推断为 Steward。复用现有 Port/Adapter、queue、storage、Readiness、delivery fence 和 immutable Commands；没有新 SDK、目录、权限或 workflow 基础设施，没有 migration 或 Core gate 修改。

```powershell
# 先按既有 B3 入口构建 api/worker/fixture；prefix 匹配各 binary。
python examples/enterprise-activity/postgres-ingestion/b3_runtime.py --output .data/b3-closure-production-runtime --binary-prefix pgi-b3-closure-final --metadata-url http://127.0.0.1:55691/api --metadata-token-file .data/pgi09-closure/token
# runtime.json 的 private/test.json 只在本机供 fixture 使用。
# rights、浏览器实际加工/Quality/Compliance/Certification/下载后：
.data/pgi-b3-closure-final-fixture.exe release-healthy
# healthy capture 后 docker stop 同一 isolated server；outage capture 后：
.data/pgi-b3-closure-final-fixture.exe release-outage
# 等真实 FAILED，capture；docker start 同一 server，等待原 worker retry 后 capture。
python examples/enterprise-activity/postgres-ingestion/closure_verify.py --runtime .data/b3-closure-production-runtime/runtime.json --phase recovered --previous examples/enterprise-activity/postgres-ingestion/results/2026-10-11/closure/history-projection-failed.json --output .data/recovered.json
# 实际撤销/替代认证/再次下载及撤销由当前可信 fixture/API/浏览器入口执行。
python examples/enterprise-activity/postgres-ingestion/b3_verify.py --runtime .data/b3-closure-production-runtime/runtime.json --download <真实浏览器下载文件> --metadata-status OBSERVED --output .data/b3-report.json
```

真实权限与 finalization 均由现有 Commands 校验。publication fixture 仅选择本 workspace/workflow 下唯一一个真实成功、READY 且 Quality/Compliance PASS 的浏览器产物；不能 SQL 插入 PUBLISHED、改变状态或放宽 Readiness。两份发布由既有生产 worker 投影，未伪造外部成功。

## 浏览器证据

- [完整治理字段与 Core 属性](results/2026-10-11/closure/governance-recovered.jpg)、[未指定 Steward](results/2026-10-11/closure/steward-unassigned.jpg)、[断连与失败刷新](results/2026-10-11/closure/governance-outage.jpg)。
- [三份 RAW](results/2026-10-11/closure/raw-accepted.jpg)、[隔离原值](results/2026-10-11/closure/quarantine.jpg)、[质量失败拒绝认证](results/2026-10-11/closure/quality-rejected.jpg)、[实际认证](results/2026-10-11/closure/certified.jpg)、[显式替代](results/2026-10-11/closure/certification-superseded.jpg)。
- [断连期间非空历史](results/2026-10-11/closure/publication-during-outage.jpg)、[恢复后历史](results/2026-10-11/closure/publication-recovered.jpg)、[撤销后阻断](results/2026-10-11/closure/revoked-again-blocked.jpg)。

主机曾在早期开发试跑中途重启；该中断不算应用故障恢复验收。最终报告只引用随后全新正式模式环境的业务事实与显式 server stop/start 实验。原有 B1/B2/PGI09 报告保持各自历史范围；最终报告没有冒充新一轮 B1/B2 或生产验收。

人工非开发者解释、生产 IAM/provider admission、真实客户数据及生产灾备均 **NOT_RUN**。目录恢复不恢复撤销授权；没有借故障恢复重新生成或改写 Core 冻结事实。
