# ADR-0007：行业专属语义放在行业包（Industry Pack）中

- 状态：已接受

## 背景

第一个实现采用园区场景，但核心平台必须能跨制造、政务、金融、医疗等领域复用。

## 决策

行业专属语义以规则、模板与配置的形式打包在 `industry-packs/<industry>/` 下。

行业包可以定义：

- Entity Types
- Glossary / Domains
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Product Templates

核心领域代码中不得散落 `if industry == PARK` 之类的行业分支判断。

## 后果

- Park 只是一个参考实现（Reference Implementation），并非产品的边界。
- 新行业可以复用核心的生命周期、版本、权利、成本与证据模型。
- 随着生态成长，行业包的契约与校验需要显式化。
