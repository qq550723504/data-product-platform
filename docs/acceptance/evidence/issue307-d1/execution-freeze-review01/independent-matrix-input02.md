# #307 D1 分支冻结修复验收矩阵

本切片修复授权后 PR HEAD 继续更新的故障。有限 contract 见 [执行与分支边界](issue307-d1-execution-freeze-boundary.md)。#332 保持 OPEN / Draft / unmerged；#307 保持 OPEN。本轮对象存储请求、真实删除、Source 启动、预算消耗、pins 释放均为 0。

## 真实故障与保留

v29 execution a5d1e569-f80e-4b38-8f32-d8139a30a077 已终态，授权 HEAD caab46da8761e71e1d80d0c0931707a410c13a4d 在授权后被 GitHub merge-main 推进到 17c5248a868376603ae9c3951123c545f2f7e56f。窗口门禁正确停止，但缺少阻止更新的原生分支锁，直到窗口才发现漂移。

旧执行保留 FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE 的实际失败与未运行事实；IAM 关闭确认仍为 UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START，不补写 PASS。封存详情以 [v29 窗口矩阵](issue307-d1-v29-window01.md) 及原 terminal 为准。本轮将 124 份原授权、调度、核对和失败回执按原字节复制入本修复 checkout，逐项 SHA/长度核对通过；原 checkout、私有控制目录、21 份旧预算、3 份本代空预算、pins 和 57 份备份均未修改。这里引用原封存事实，不冒充本轮重新访问 native/IAM 的结果。

## 验收矩阵

| 项目 | 本轮结果 | 证据与边界 |
| --- | --- | --- |
| 根因及更新边界 | PASS | 最终提交/CI 后原生锁定；锁后回执追加保存，禁止推进冻结 HEAD；终态不自动解锁 |
| GraphQL 最小修改 | PASS | 仅修改 exact rule 的五个冻结字段；非冻结保护完整读回；shared/wildcard rule 拒绝 |
| 竞态和不确定结果 | PASS | 锁前/后 HEAD 与完整保护核对；无原子 CAS 声称；API 未知或回执写失败不重试、不自动解锁 |
| 固定完整入口 | PASS | 实际授权存在后必须传入新 execution-binding SHA；缺失/漂移在 Docker/Source 前停止；audit-only 仍仅离线 |
| Host.check / H1 / attester | PASS | 共用实际授权与原生 fresh branch gate；分支拒绝早于 inspect/SQL/签名；seal 不依赖有效授权或分支锁 |
| CI 来源 | PASS | 精确 HEAD、仓库、PR332、ci.yml、pull_request、最新 run/attempt、GitHub Actions app、suite 与完整 job/check 成员一致；不能用同名 required 冒充 |
| 正常 / -O 回归 | PASS | 13 freeze + 8 complete entry + 7 support + 4 attester + 1 generator，各模式 33 项，均成功；Docker/SQL 使用显式 doubles，真实 Ed25519 签名 |
| 原生 GitHub 写锁 | PASS | 独立 rehearsal 分支实际锁定；同 tree 的合法快进 commit 更新被 422 Cannot change this locked branch 拒绝；分支和锁保留 |
| 修复后原生 CI 来源读回 | PASS | 对保留的 rehearsal 分支仅重新读回；run39715490720、13 个 job/check 成员成功；无再次更新尝试 |
| 独立复评 | PASS | B1/B2 均闭合、剩余有效阻断 0；独立正常/-O 各 33 项通过，13 个审查输入前后 SHA 相同；原生响应离线重放及 6 个 CI 来源反例通过；初评失败和反例保留 |
| 本修复最终 HEAD 的 PR CI | PENDING | 等待提交推送后的精确 HEAD pull_request 结果；既有 17c HEAD CI 不能替代 |
| 新候选与实际 #332 执行锁 | NOT_RUN | 本轮未分配 execution/预算/窗口，未生成或冻结候选；实际 #332 仍处准备阶段 |
| 新的人类授权 / 调度 / 删除 | NOT_RUN | 旧授权绝不复用；六目标全部保留待执行 |

本轮证据位于 evidence/issue307-d1/execution-freeze-review01：regression02/result.json、native-rehearsal01/native-result.json、native-ci-provenance-readback02/readback.json、retained-v29-receipt-index.json。原生演练只证明 GitHub 分支写保护，不代表对象存储删除、IAM 关闭或完整 D1 验收。

## 后续新方案的有限顺序

先完成本修复独立复评和精确 HEAD PR CI。随后为剩余 P2/T1/T2/H2A/M1A/M1B 准备新 execution、独立新预算与新固定窗口，重新做 17 版本只读核验、非删除验收和冻结评审；P1 永久排除，11 survivor 和历史全部保留。候选源码、文档、评审及静态计划最终提交并通过 CI 后，使用 branch-freeze.py 锁最终 #332 HEAD，锁后 fresh 读回并产生 execution-binding.json；完整新确认必须额外包含其 SHA、最终 HEAD、ruleID 与完整保护绑定。缺少这些事实时方案只能标准备中，不能请求删除授权或建立执行调度。
