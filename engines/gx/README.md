# GX Core 质量参考引擎

这是 `QualityEnginePort` 的首个外部 reference runtime。它使用 **Great Expectations / GX Core 1.23.2** 执行基础表格质量规则，但不拥有 QualityAssessment、Finding severity、GateDecision、Evidence 或 Certification。

## 边界

Core 负责冻结 DatasetVersion 与 RuleSet、分配 AssessmentAttemptID、验证 engine capability、归一化 provider payload、推导最终 GateDecision 并持久化事实。

GX runtime 只负责同步执行规则并返回 provider-neutral observation：

- `not_null`
- `completeness_ratio`
- `unique`
- `duplicate_ratio`
- `range`
- `enum`

运行时是无持久状态的同步服务，不创建 provider-side authoritative job。请求携带稳定 attempt ID 仅用于诊断关联；超时/传输失败由 Core 记录为 execution failure，而不是 quality rule FAIL。

## API

```text
GET  /health
POST /v1/evaluate
```

`POST /v1/evaluate` 接收冻结 RuleSet 文本与当前 DatasetVersion 的表格行，返回规则级 PASS/FAIL 和受限 observation。provider traceback、数据样本和自由文本不会作为响应事实返回。

## License / version

reference runtime 锁定 `great-expectations==1.23.2`，adapter contract 版本为 `1.0.0`，对外 engine version 为 `1.0.0+gx.1.23.2`。adapter 的解析、归一化或规则映射语义变化时必须提升 contract 版本；GX 依赖升级也会改变组合版本。GX 1.23.2 的 PyPI 元数据标识为 Apache-2.0。

## Run

```bash
pip install .
uvicorn app.main:app --host 0.0.0.0 --port 8092
```
