# #307 D1：candidate05 / v15 授权执行验收

2026-10-16，**两份获批源版本各一次 exact DELETE，随后只读恢复确认 absence；四份 survivor 完整保留，最终账务和历史审计 PASS。** execution `9ea8f3e2-c805-44b4-b133-a36f50df9d15` 的发送额度已全部使用，不得重发。#318 保持 OPEN / Draft，不合并；cleanup=0、hold-release=0、bypass=0。

[实际验收回执](evidence/issue307-d1/delete-confirmed-v15-execution02/post-seal-audit03/acceptance.json)、[完整证据索引](evidence/issue307-d1/delete-confirmed-v15-execution02/evidence-index.json)及[冻结方案](issue307-d1-candidate05-confirmation.md)共同解释本次结果。该结论仅覆盖获批 provider-accessible exact versions；全部备份、其他环境及外部接收方不在删除范围内。

## 授权、冻结绑定与窗口

[新授权回执](evidence/issue307-d1/delete-confirmed-v15-execution02/human-authorization.json)如实保存用户原文：用户明确引用已核对的 candidate05/v15 完整方案及全部绑定授权，没有声称用户重新逐字输入模板。私有确认 envelope 包含该原文和获批模板，其 SHA 同时绑定新 BasisRef 和两份 permission。旧 `614cd9ad` 授权、窗口及发送额度未复用。

project `dpp-lifecycle307-5d2274ee`、provider `1980751b-c50e-4895-93a8-cb55960cd635`、incarnation `be784eda-96e4-49a9-ae47-eaa3203a2dc5`、plan `8f0e9340-18f0-4a73-bdd6-622196a9011a` / hash `d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc` 均与[获批 source-binding](evidence/issue307-d1/delete-candidate05-preparation01/source-binding.json)一致。原 Core 检查点 `85567273b268e66511ab58d6c03a430788b3f93ceb7346a52cf44eaa82939ab6` 及 dump/globals 原字节保留。

实际使用冻结 source `dbe3fbd666146db6a010a508fa99e6ed558b6fe9`，Linux sender `4614fe539f2a41a39fa3d07bf0adf10f832fcc7dff04abf6e9f538617a21b4cd`；Windows attestor、Linux recovery、profile、target digest、IAM、active/sealed gateway 及 317 项 runtime LF hash 全部保持获批绑定，没有替换 sender/host/recovery binary。

两 sender 共用固定 **01:30:22.752003Z～02:05:22.752003Z，35 分钟**窗口。01:34:36Z 激活，01:35:12Z 完成原始封存，未延长窗口。后续数据库审计保持 IAM、sender、recovery 和 gateway 关闭，没有 native provider I/O。

## 真实执行与精确 absence

| 目标 | 精确源版本 | 一次物理 attempt | 只读 absence receipt |
| --- | --- | --- | --- |
| A | `dpp-lifecycle307-online/fixture/a.csv@312365a9-f984-43de-b9d5-7e26009ccd82` | `8c39da56-f252-4a57-bb45-8fc685ec2049` | `d3340b85-3ac6-46ff-983b-e3330d28a54d`，01:34:40.893014Z |
| B | `dpp-lifecycle307-quarantine/fixture/b.csv@846d0c7c-289d-4667-bc7c-aa04e44357d4` | `7cd82536-534e-4f69-98c9-1e335ef8c34c` | `1ce3b0b7-9dd7-4f7f-906b-a7e2ee25e461`，01:34:47.133551Z |

每次 native DELETE 经 frozen gateway 的成功响应丢失路径，sender 提交 `CONFIRMED/UNKNOWN` observation 和 invocation cost 后退出 93。结果由既有只读 recovery 确认，未按 UNKNOWN 重发。实际 outbox obligation 经 dispatcher / Asynq worker 执行；精确 WorkID 为 A `7a56e035-d1a9-44d1-84f4-13d8d6fbcabe`、B `6561381a-d1e4-4c01-ae11-a26eeebe6053`。非目标 outbox 行锁隔离、事务 ROLLBACK，旧业务义务未被消费或改写，没有直接注入队列。

[持久化时序和成本审计](evidence/issue307-d1/delete-confirmed-v15-execution02/post-seal-audit03/acceptance.json)确认 A absence 已提交才创建 B attempt。A 后为 1 receipt / 0 completion，B 后为 2 receipts / 1 completion；completion `f5b914ef-f6b6-4e53-a883-45d66e45d45c` 于 01:34:47.219960Z 提交。两次 terminal replay 均没有新 provider I/O、claim、absence 或 completion。

## 验收矩阵

| 验收 | 实际结果 |
| --- | --- |
| 新授权与冻结绑定 | PASS：新 execution、envelope、source/binary/profile/IAM/gateway/checkpoint 全部精确绑定；无旧授权复用 |
| 删除前 Core / reference / owner | PASS：源 194 表 / 2361 行及 role/ACL 不变；27 qualified guards、54 target FK、23 requirements/releases、7 terminated pins、0 pending copy、2 ready outputs |
| 删除前 native / IAM | PASS：六版本 bytes/SHA/hold/retention/完整库存相同；20 个新只读调用记在原检查点 clone，165→185；源仍125；5次管理GET、0源业务写入 |
| 新受限 IAM | PASS：SDK enabled-create → attach → disable，原生 exact policy 回读匹配；发送前有限启用，最终独立 disable 回读；reader/producer/旧 executor 未重新启用 |
| root-free 发送 / fresh gates | PASS：two bind mounts + tmpfs；fresh signed host topology、共享 fence 下 dispatch / ClaimExactSend；A/B 各一个新 claim |
| 成功 response-loss / recovery | PASS：两次 CONFIRMED/UNKNOWN + exit93，2 个实际 worker terminal result；A absence 后才启动 B；无重发 |
| 四 survivor / 库存 | PASS：下表四份 exact bytes/SHA/hold/retention 与本轮 preflight 一致；最终 online3 + quarantine1，无额外版本或 delete marker |
| 成本与历史 | PASS：源 calls / observations / typed storage costs 125→157；32 新读调用与2新 DELETE 均独立记账，quantity=1 invocation；旧 provider cost1及全部历史保留 |
| Core 旧行与新命令 | PASS：同一 bound dump 在新隔离 audit Core 恢复为194表/2361行；当前2851行中旧行完整保留，只有共享 fence projection 90→97；7新命令 key/type/subject/result 全部绑定本轮 |
| Domain Event / Audit / Evidence / Outbox | PASS：本轮新增81条对应事实及Evidence关联；旧行及旧outbox状态完整保留；原有DB约束和受控Commands生效 |
| 关闭与保留 | PASS：新 IAM disabled；两个permission、recovery及preparation关闭，gateway为获批sealed hash；113实际容器全停止/PID0/noports，26份旧 seals/backups和原dump/globals原字节不变 |
| 独立评审 | 发送前源码复评PASS、blocker0；最终有限离线复评见本目录新增独立回执 |
| PR 状态 | OPEN / Draft；未合并。PR CI另按最终提交SHA核对，不冒充main CI或部署 |

32 新存储调用由 sender 元数据12次、recovery inventory2次、survivor18次组成：retention8、hold8、inventory8、stat4、checkpoint bytes4。[账务闭合](evidence/issue307-d1/delete-confirmed-v15-execution02/post-seal-audit03/ledger-validation.json)逐identity核对全部157次调用、observation、typed allocation及成本。另有独立管理RPC：preflight5、provision11、enable6、emergency-disable3，不混记为业务storage成本。

| 保留版本 | bytes | 保护 |
| --- | ---: | --- |
| `online/fixture/b.csv@159e90c3-adb6-4ea3-b2e8-2804e578e86f` | 129328 | hold ON / NONE |
| `quarantine/fixture/a.csv@5a7ce790-9093-4569-a8fd-0726009a5eb4` | 232 | hold OFF / GOVERNANCE 至 `2026-10-15T00:58:47Z`，已到期仍保留 |
| `online/fixture/standardized.csv@2139f293-87f4-4d49-a9f0-941b22275093` | 494 | hold OFF / NONE |
| `online/fixture/processed.csv@7c8ad93b-31ee-445c-9402-816dda0410ec` | 232 | hold OFF / NONE |

完整 SHA 见[四 survivor 原生回执](evidence/issue307-d1/delete-confirmed-v15-execution02/survivors/four-survivors.json)；namespace 前缀均为 `dpp-lifecycle307-`。

## 原停止回执与审计收口

原操作器在真实删除和四survivor读取完成后，其最终 relation allowlist 漏掉正常的 `command_idempotency`、`evidence_relation`，并误用了 `workspace_delivery_fence` 名称，因而停止并独立封存。[原停止回执](evidence/issue307-d1/delete-confirmed-v15-execution02/execution-stopped.json)和原源码原样保留，没有把原 execute 退出码改称 PASS。

后续单独只读审计从获批原始 dump 恢复独立 audit Core，验证全部194表旧行子集，解释新增命令及共享 projection；最终 source 无新变更、provider I/O=0。[最终审计](evidence/issue307-d1/delete-confirmed-v15-execution02/post-seal-audit03/historical-subset-check.json)按实际 contract 核四个新 execution 前缀命令、两个 `recovery-absence:<runID>` 和一个 `recovery-finalize:<planID>`；dispatch 命令结果强类型关联 evaluation ID，再连接实际 dispatch / attempt。前两次只读审计的预期修正及关闭回执同样保留，没有改写业务历史。

原封存清单131条记录混用短/完整容器ID，规范化后覆盖112实际容器；加上后续独立只读 audit Core，最终为[113个完整64位ID的已封存容器](evidence/issue307-d1/delete-confirmed-v15-execution02/final-topology-closure.json)。原清单没有覆盖或删除，所有旧资源和新审计副本均保留。

本轮完成 #307 D1 此精确方案的原生成功删除与崩溃恢复验收；#318 继续保持 Draft。真实删除授权已消费完毕，不构成后续删除或合并授权。
