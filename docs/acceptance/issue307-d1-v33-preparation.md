# #307 D1 candidate23/v33 准备与有限验收矩阵

本代只准备，不执行真实删除。#332 OPEN/Draft、不合并；#307 OPEN。candidate22及旧预算、pins、失败和UNKNOWN事实保持原字节，不复用任何旧授权。

## 有限交付与权威边界

目标：为剩余P2、T1、T2、H2A、M1A、M1B六份exact版本准备新候选和独立预算；P1永久排除，11份survivor保留。权威目标仍来自原冻结remaining scope及当前实际Core；准备文档不授予DELETE。

权威过程沿用 `issue307-d1-execution-freeze-boundary.md`、`fixture_artifacts.py`、`prepare-six-candidate.py`、`six_candidate_runtime.py`、`freeze-six-candidate.py`、`verify-six-native.py` 和 `window_six_support.py`。不引入第二套队列、策略引擎或工作流。

本切片invariants：旧事实/已用预算不得覆盖或退款；全部已知owner必须实际关闭；预算归属和完整挂载集合精确；新R和P分开固定；只有精确P的PR CI成功后才允许分配；最终HEAD及完整保护重锁后才能形成执行绑定。没有删除授权或定时任务。

非目标：资源回收、已保留容器删除、schema升级、Rights/Renew、Processing/DirectData重跑、恢复旧授权、合并和D1整体验收。

## 新代计划

- execution：`4fad1007-6595-4037-a7a5-1d8d38cb8bcc`。
- profile：`candidate23/v33`。
- 拟定窗口：2026年10月24日新加坡18:00–18:40，即UTC `2026-10-24T10:00:00Z ≤ now < 2026-10-24T10:40:00Z`；只读恢复不晚于UTC12:40。
- 窗口由用户“自行准备”后拟定，只是准备参数。冻结后仍需新的人类精确删除授权；调度另行授权。不得自动延长或把旧窗口迁移到本代。
- R源码：`6255b62b94ffdb2453b89c7caaa777e69cc6a6d8`，直接基于原R `81a083751c01a281d42df594f6e91012255081f1`，仅五个已列出scope/policy/测试文件变化。
- R ZIP：`0936e6cd685134d9b850b1a770e097bf3632e56a547b12460bdcb4b5ebe78e02`，6428279 bytes，769个运行归档成员；保留schema69/66与无protected identity约束。
- 运行二进制：NOT_BUILT；新预算：NOT_ALLOCATED；新candidate/bindings/template：NOT_FROZEN；最终HEAD/execution-binding：NOT_BOUND；删除/调度：NOT_AUTHORIZED。

## 当前验收矩阵

| 检查 | 状态 | 事实与限制 |
| --- | --- | --- |
| 新execution、统一END与recovery期限 | PASS_LOCAL | 新scope只在后继工作区修改，旧C22不改 |
| 保留C22六个真实IAM身份及完整原双action策略 | PASS_LOCAL | Go R与P相关五个package通过；不是native IAM证明 |
| R隔离归档与原生Git UTC/store配方 | PASS_LOCAL | 同CI脚本本地归档及Go测试通过；不是GitHub PR CI |
| 完整入口/窗口/预算/冻结/证据回归 | PASS_LOCAL | Python normal与-O各108项，1项按原条件跳过；后续只对改动的原字节保护回归复验 |
| DNS null/[]有限历史比较 | PASS_LOCAL | 两份原SHA固定plan的725个CID；仅空DNS单字段解释，完整原配置SHA必须一致；不改inspect/State/挂载 |
| candidate22全量原件、关闭状态与40份旧预算 | PASS_READONLY | capture03及实际prepare-six-candidate.historical_gate均通过：1085 closed CID、40份预算、75备份、5364公共原件；anchor为6af1a5bf3ce4a4f599ca1de4cc7f81f8681e065123fb1e65cd9420fd3f167f2f；不将原C22完整入口重标PASS |
| capture01/02失败与原字节保护 | FAILED_RETAINED / FIX_VERIFIED | 新checkout的四份C21和三份C22已发布Markdown被CRLF转换；两次失败和转换前字节均保留，仅对照原索引SHA和LF相等恢复本工作区，并补精确-text规则；旧原件未变；补后normal/-O各9项通过 |
| #332保护 | FROZEN | 当前远端HEAD仍8de3bc4c9f737c3c3e81fddb60491422ba6039f2；lockBranch/enforceAdmins=true，其余冻结字段维持；未解锁 |
| C23 P精确PR CI | NOT_RUN | 旧C22绿色不能代替本代；新源码尚未推到#332 |
| 独立新预算与新资源分配 | NOT_RUN | 必须在P精确CI后；不复用旧预算或删除旧环境 |
| 17版本全量原生只读核验 | NOT_RUN_C23 | 原C22读回仅保留为历史；不冒充本代fresh证明 |
| 新source/binary/Core/IAM/gateway绑定冻结 | NOT_RUN | 只能从实际新代结果生成 |
| 源码独立评审 | IN_PROGRESS | B1授权缺失严格判断、B2旧锁后原件先验SHA已修；独立复核已确认实际capture/anchor/补后回归，待精确本地P提交终核与报告 |
| 最终候选独立评审/精确确认文本 | NOT_RUN | 未冻结前不输出可执行确认模板 |

## 有效阻断与下一步

1. 保留capture01/02原失败、capture03成功和固定完整保留入口的实际PASS；本轮source_start=0/native_delete=0/pins_released=0/budgets_refunded=0。
2. 完成本地源码独立评审及提交，形成可审查的P；C22专用分支维护授权不能复用于C23。具体维护边界见candidate23-planning01/maintenance-proposal01.json。
3. 取得新的受限维护授权，实时核保护后只解除#332的lockBranch位，提交本代源码，等待精确HEAD PR CI。其他保护保持。
4. 精确P CI通过后，才创建独立execution预算/资源、运行非删除原生验收和17版本核验；独立评审、冻结全部实际绑定、最后CI通过后重锁分支并生成新execution-binding。
5. 冻结最终完整方案交用户核对；本轮没有真实删除授权。

## 设计观察

当前准备链把历史证据保留与每代运行容器的存活依赖放在一起，已造成重复检查和数量口径膨胀。本轮明确40份实际旧预算并保持原保护，不把资源删除或归档迁移变成新候选的隐含前置。独立资源治理需要另行有限设计、证据和授权。

本矩阵记录本地源码准备事实；精确本地P提交号由提交后的独立评审另存，避免把包含自身commit的哈希写入tracked文档。完整保留入口不是17版本原生内容/IAM核验，后者仍为NOT_RUN_C23。
