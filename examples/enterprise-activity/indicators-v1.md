# 企业活跃度指标规范 V1.0

本文档是以下文件的人类可读配套说明：

`industry-packs/park/indicators/enterprise-activity-v1.yaml`

V1 指标刻意设计为透明且基于规则，使 POC 能够验证版本管理、可解释性、Quality/Compliance 门禁、证据与发布可复现性。

> **重要提示：** 这些得分是 POC 产品指标。它们不是经过验证的银行信用评分、授信决策、违约概率模型，也不是投资/风险建议。真实的金融用途需要领域负责人定义、真实数据校准、验证以及消费者认可。

## 1. 目标期间

参考实现按权威企业（canonical company）与目标月份（`YYYY-MM`）计算一行输出。除规则另有说明外，日期均以目标期间结束日为准进行评估。

所有得分在计算完未圆整的指标值之后，使用 HALF_UP 四舍五入到 2 位小数。

## 2. `tenancy_stability`

目的：提供一个可解释的信号，说明企业存续时长，以及是否存在覆盖目标月份的生效租约。

输入：

- `enterprise.entry_date`
- `lease.contract_start`
- `lease.contract_end`
- `lease.lease_status`

V1 参数：

- 完整存续期基准：36 个月
- 存续时长权重：70%
- 当前生效租约权重：30%

计算：

```text
tenure_months = 从 entry_date 到目标期间结束日的完整自然月数
                并截断到 0..36

tenure_score = min(100, tenure_months / 36 * 100)

current_lease_score = 100
  若至少存在一份覆盖目标期间的 ACTIVE 租约
  否则为 0

tenancy_stability = 0.70 * tenure_score
                  + 0.30 * current_lease_score
```

缺失数据规则：

- 缺失 `entry_date` → 指标为 `null`
- 无可用租约数据 → 指标为 `null`

实现必须保留用于解释的分项：`tenure_months`、`tenure_score`、是否找到生效租约，以及 `current_lease_score`。

## 3. `rent_performance`

目的：汇总缴付事件的表现，同时不掩盖逾期或未缴事件。

输入：

- `lease.payment_due_date`
- `lease.payment_date`

V1 窗口：

- 截至目标期间结束日的近 12 个月
- 至少需要 2 个到期事件

事件得分：

| 事件 | 得分 |
| --- | ---: |
| 在到期日或之前缴付 | 100 |
| 逾期 1–7 天 | 70 |
| 逾期 8–30 天 | 40 |
| 逾期超过 30 天 | 20 |
| 截至目标期间结束时逾期未缴 | 0 |

```text
rent_performance = 各事件得分的算术平均值
```

尚未到期的事件在其到期日到来之前不计入。

如果可用的到期事件少于 2 个，结果为 `null`（该分项为 `INSUFFICIENT_DATA`）。

解释信息应保留按分类统计的到期事件计数。

## 4. `energy_stability`

目的：衡量有效能耗观测的连续性与逐月稳定性。它刻意不宣称能耗上升/下降对信用风险是好是坏。

输入：

- `energy.reading_time`
- `energy.energy_kwh`

预处理：

1. 对 POC 而言 `energy_kwh < 0` 在技术上无效，会被**隔离**，绝不改写为零。
2. 被接受的读数按权威企业汇总为月度总量。
3. 使用截至目标月份的近 6 个月。

最小数据要求：

- 至少 3 个有效月度总量
- 有效月度总量的算术平均值必须大于零

期望月份数从回溯窗口内第一个观测月份起计至目标月份，并上限为 6。这避免了仅仅因为 POC 数据集晚于六个月窗口才开始，就对某企业产生惩罚。

```text
coverage_score = 有效月份数 / 期望月份数 * 100

cv = 总体标准差(有效月度总量)
     / 平均(有效月度总量)

variability_score = clamp(100 * (1 - cv / 0.30), 0, 100)

energy_stability = 0.70 * variability_score
                 + 0.30 * coverage_score
```

`0.30` 是 POC 参考 CV 阈值，刻意保存在版本化配置中。在做出生产级结论之前必须对其进行校准。

解释输出应保留：

- 期望/有效月份
- coverage 得分
- 月度均值/标准差
- 变异系数（coefficient of variation）
- variability 得分
- 被隔离记录数

## 5. `activity_score`

V1 要求三个分项指标全部可用：

- `tenancy_stability`
- `rent_performance`
- `energy_stability`

初始合成使用等权重，以避免暗示某种经实证验证的风险权重：

```text
activity_score = 1/3 * tenancy_stability
               + 1/3 * rent_performance
               + 1/3 * energy_stability
```

如果任一分项为 `null`：

```text
activity_score = null
activity_level = INSUFFICIENT_DATA
```

**不存在静默的零值、中性得分、中位数或前值填充（forward-fill）补插。**

POC 活跃度等级：

```text
HIGH   >= 80
MEDIUM >= 60 且 < 80
LOW    >= 0 且 < 60
activity_score 为 null 时为 INSUFFICIENT_DATA
```

这些等级阈值是 POC 展示语义，不是经过验证的金融阈值。

## 6. `indicator_coverage`

产品暴露一个简单的可解释性/可用性度量：

```text
indicator_coverage = 可用的必需分项数 / 3 * 100
```

示例：

- 3/3 可用 → 100
- 2/3 可用 → 66.67
- 1/3 可用 → 33.33
- 0/3 可用 → 0

覆盖率低于 100 意味着在 V1 中 `activity_score` 为 null。

## 7. 质量与业务指标语义的区别

`activity_score` 描述参考产品的业务指标输出。

它**不得**被复用为数据质量（Data Quality）得分。数据适用性由 Quality Rules / Quality Gate 独立评估。

一个企业的 `activity_score` 可以很低，而其 Dataset 质量可以极好；反之，一个计算出的高活跃度得分，如果数据质量不通过，仍可能被阻止发布。

## 8. 版本管理与证据

每一次产出这些指标的执行都必须保留：

- 指标集名称/版本
- 确切的输入 DatasetVersion
- 工作流版本
- 实体匹配策略版本
- 被隔离/无效记录计数
- 分项解释值

已发布的 ProductRelease 会在其证据快照中冻结这些引用。
