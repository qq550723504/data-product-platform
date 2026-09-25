# Gold External Explanation Test

Issue: #208  
Purpose: 人工证伪 Gold Dataset explainability / audit UX。  
Required participant: 至少 1 名**未参与该链路实现**的人。

## 1. 测试前准备

测试主持人选择一条已完成的 Gold DatasetVersion。

受试者只能访问产品 UI，包括：

- DatasetVersion / Gold detail
- Quality report
- Certification history
- Current Delivery Eligibility
- Gold production proof
- Campaign / Review explanation
- Cost / Evidence / Audit trace

不得提供：

- SQL
- 源代码
- 内部 ADR / 架构文档
- 数据库后台
- 开发者补充说明

## 2. 记录信息

- 日期：
- 受试者角色：
- 是否参与过 Gold 实现：必须为“否”
- Gold DatasetVersion ID：
- 主持人：
- 使用环境 / commit：

## 3. 问题与判定

每题记录：

- 回答摘要
- UI 使用路径
- 是否独立回答
- 是否准确
- 是否需要主持人解释
- UX / trace 缺口

| # | 问题 | PASS 条件 | 结果 |
|---|---|---|---|
| 1 | 数据来自哪些输入和版本？ | 能指出 input DatasetVersion / lineage | |
| 2 | 经历哪些实际生产步骤？ | 能解释 annotation -> review -> snapshot -> build -> quality -> certification -> delivery | |
| 3 | 哪些决定由自动引擎产生？ | 能识别 annotation engine / automated facts | |
| 4 | 哪些决定由人工完成？ | 能指出 reviewer、decision、reason | |
| 5 | 谁完成 Annotation，使用什么 schema/taxonomy？ | 能从 frozen campaign/provider refs 回答 | |
| 6 | Review 最终选择哪个 immutable result？ | 能指出 selected result / provenance | |
| 7 | Gold Quality 为什么 PASS/FAIL？ | 能引用 rules/findings/metrics | |
| 8 | 为什么获得 Gold Certification？ | 能说明 exact quality + rights + production proof | |
| 9 | 当前为什么允许/拒绝交付？ | 能解释 CurrentDeliveryGate / current entitlement | |
| 10 | 各阶段有哪些 Cost/Evidence/Audit？ | 能从 phase trace 回答 | |

## 4. 总体 PASS 标准

Pilot External Explanation Test 通过需要同时满足：

- 10 个问题全部能仅凭产品界面回答；
- 来源、人工决定、质量、认证、当前交付资格五类核心问题不得依赖开发者口头解释；
- 未出现必须阅读 SQL / code 才能理解的关键事实；
- UUID/hash 可以作为 proof reference，但 UI 必须提供足够语义说明；
- Cost / Evidence / Audit 能关联到业务阶段而不是只显示孤立技术 ID。

任何核心问题无法独立回答，则本次测试 FAIL。

## 5. 发现记录

### 无法回答项

-

### 容易误解项

-

### UX / trace 缺口

-

### 建议改进

-

## 6. 测试结论

- [ ] PASS
- [ ] FAIL

结论说明：

## 7. 对 Pilot 报告的更新

测试完成后必须同步：

`docs/poc/gold-dataset-pilot-acceptance.md`

更新：

- External Explanation Test 结果
- 受试者角色
- 无法回答项
- UX/trace 缺口
- 最终 Pilot 结论

不得因为自动化 CI 全绿而跳过此人工测试。
