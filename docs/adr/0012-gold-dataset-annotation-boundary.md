# ADR-0012：Gold Dataset 与标注引擎的业务边界

- 状态：拟接受，随 #209 架构文档 PR 合并生效
- 日期：2026-09-23
- 关联：#203、#209；实现 #204–#208
- 补充：ADR-0003、ADR-0004、ADR-0005、ADR-0006、ADR-0010；不替代第一阶段历史验收

## 背景

Certified Dataset 第一阶段已验证受控生产、质量、权利、认证与 DIRECT_DATA。第二阶段需要把外部标注和独立人工审核纳入同一可解释生产链。如果直接以引擎项目状态或导出文件作为 Gold 真相，外部修改/删除会改变历史；若另建一套 GoldDataset、质量和交付模型，又会产生第二套事实和授权路径。

因此需要区分通用标注执行能力、Core 对结果的业务接纳、不可变生产证据与当前使用/交付权限。

## 决策

### 1. Core 拥有业务事实，引擎拥有执行能力

Campaign、Task 的 Core 身份、已接纳 Result、ReviewDecision、AnnotationSnapshot 及其 Gold 生产/认证关联由 Core 持有。外部标注产品通过 Annotation Engine Port / Adapter 接入，补充 ADR-0004 的引擎边界，不直接依赖其 SDK、项目状态或数据模型。

只承诺已被 Core 捕获并保存的事实可验证，不声称能恢复尚未捕获就被外部覆盖的历史。Provider ID 是外部绑定，不代替 Core canonical identity。

### 2. Gold 不是第二套数据集主实体

Gold output 仍是 immutable DatasetVersion。它相对于明确 Profile，额外具有 FINALIZED AnnotationSnapshot、实际生产依赖、规范版本与质量/认证证明；不创建独立 GoldDataset aggregate，不复制一套 Delivery 或 Quality 状态机。

是否为 Gold 认证由所引用的 Profile 和证据决定，不通过可变标签或客户端参数决定。

### 3. 修正追加事实，不覆盖历史

标注修订形成新 Result；人工 CORRECT 保存原结果、纠正结果和明确选择它的 Decision。Snapshot 冻结完整任务处置与结果选择，包括拒绝和未导出成员，不能只冻结通过项。后续 provider 修改不得回写已封存历史。

权威结果由明确的不可变审核决定选择，不由 latest timestamp、任意匹配项或 provider ground_truth 推断。并发 mutation 与 finalization 必须在同一 parent/fence 边界线性化。

### 4. 生产证明与当前权限分离

Gold 生产必须绑定实际输入、AnnotationSnapshot、规范和 producer，并冻结足以解释该输出的完整依赖。独立交付不强制创建 ProductRelease，因而不能依赖未来 ProductRelease publication 才保护 Gold 历史。

源数据及标注贡献都是权利依赖。Effective Rights 基于完整依赖重新证明，不能因完成标注或已有认证而自动获得新增权限。每次实际使用/外发/交付仍校验其当前权限；历史快照不是永久授权。

### 5. 外部未知结果不能伪装成原子成功或确定失败

外部副作用与 Core 数据库不是同一事务。先保存稳定 operation/physical attempt，再调用；重放和恢复采用明确身份与可验证观察。没有提供者支持时，不声称能同时实现盲目自动重试和远端 exactly-once。无法证明的结果保持未决/人工处置，不通过重复创建隐藏不确定性。

## 备选方案与取舍

| 方案 | 不采用原因 |
| --- | --- |
| 直接把 Label Studio 当前项目/导出作为 Gold 真相 | 外部可变状态不能满足历史、权利、认证和替换边界 |
| Fork 标注系统并复制 Core 规则 | 放大升级/维护成本，未证明原生 API 与 Adapter 不足 |
| 自建通用标注画布、分发、审核/BPMN 平台 | 超出最小纵向 Pilot；成熟通用能力应复用 |
| 独立 GoldDataset / GoldDelivery 主模型 | 重复 DatasetVersion 和当前授权机制，容易形成绕过路径 |
| 只留文件 hash 和 provider URL | hash 不等于保留了可重建的内容，不能证明完整 membership |

## 后果

收益是可解释的审核来源、冻结生产历史、统一认证/授权以及可替换的引擎。代价是需要保存规范化 payload、精确 membership、typed production/rights dependencies，并验证引擎关联查找、修订与恢复能力；在 provider 不能证明结果时，安全优先于自动恢复的可用性。

本 ADR 不冻结具体表名、HTTP endpoint、SDK/引擎 JSON、单次任务上限或多轮审核产品策略。Pilot 的有限约束、实施归属与验收见下列权威文档；扩展时更新相应契约，只有改变跨模块决策才新增 ADR。

## 权威实施文档

- [产品与验收范围](../product/gold-dataset.md)
- [Annotation Domain](../architecture/annotation-domain.md)
- [Annotation Engine integration](../architecture/annotation-engine-integration.md)
- [Gold production / certification](../architecture/gold-dataset-production.md)

#209 的合并只接受架构基线，不证明 #204–#208 已实现，更不批准生产上线。
