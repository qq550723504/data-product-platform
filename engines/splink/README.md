# Splink 候选引擎

本目录包含 Core Platform 用于生成概率化 COMPANY 匹配候选的可选 Python 运行时。

## 边界

Splink **不**拥有权威实体或映射。该服务接收一条归一化源记录以及当前权威参照集（canonical reference set），并返回带排序的候选实体 ID 及其匹配概率。

Core 仍然负责：

- 强标识符优先级
- 匹配策略阈值
- AUTO_MATCH / REVIEW / UNRESOLVED 决策
- 人工复核（Human Review）
- EntityMapping 持久化
- 证据与审计历史

## 运行时版本

参考运行时锁定 Splink `4.0.17`，并使用 DuckDB 后端。Go 适配器在启用候选生成器之前会执行 `/health` 能力/版本/模型检查。

本参考实现刻意不使用 Splink 5 的开发版本。

## 模型-策略绑定

统计模型只对其训练/评估所依据的匹配策略有效。参考模型绑定到：

```text
model:  park-company-v1@1.0.0
policy: park-company-match@1.0.0
```

Go 适配器在发起提供方调用之前会拒绝其他处于启用状态的策略，Python 运行时也独立强制同一绑定。这可以防止 Park COMPANY 模型仅仅因为实体类型碰巧相同就被应用到其他行业的 COMPANY 策略上。

对于锚点 COMPANY 实体，Core 将统一社会信用代码存为 `canonical_key`。运行时会把该参照字段映射到模型列 `unified_social_credit_code`，使源/参照的统计 schema 保持一致。

## 端点

```text
GET  /health
POST /v1/candidates
```

`POST /v1/candidates` 接收来自 Go Core 的提供方中立请求，并返回：

```json
{
  "engine": {
    "name": "SPLINK",
    "version": "4.0.17",
    "modelVersion": "1.0.0"
  },
  "candidates": [
    {
      "entityId": "...",
      "score": 0.87,
      "method": "FELLEGI_SUNTER",
      "metadata": {
        "matchWeight": 2.73,
        "matchKey": "0"
      }
    }
  ]
}
```

提供方异常与 Python traceback 只留在运行时日志内部；Go 适配器将传输失败映射为稳定的平台错误。

## 配置

```text
SPLINK_MODEL_PATH=./models/park-company-v1/model.json
SPLINK_MODEL_REF=park-company-v1
SPLINK_MODEL_VERSION=1.0.0
SPLINK_POLICY_REF=park-company-match
SPLINK_POLICY_VERSION=1.0.0
SPLINK_API_TOKEN=
SPLINK_MAX_REFERENCES=100000
```

该模型必须是训练好的 Splink `link_only` JSON 产物。参见 `models/park-company-v1/README.md` 与 `industry-packs/park/matching/splink-company-model-v1.yaml`。

## 运行

```bash
pip install .
uvicorn app.main:app --host 0.0.0.0 --port 8091
```

或构建随附的 Dockerfile。
