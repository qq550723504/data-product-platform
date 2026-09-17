# ADR-0001：核心 POC 采用模块化单体（Modular Monolith）

- 状态：已接受

## 背景

主要风险在于领域模型的正确性，而非水平扩展能力。在边界尚未验证之前，就把系统拆分为多个服务，会引入 RPC、分布式事务、部署与版本管理等额外复杂度。

## 决策

将第一个核心平台构建为模块化单体，具有清晰的领域模块，并配一个独立的 worker 进程。

初始模块包括 resource、dataset、entity、workflow、rights、quality、compliance、contract、product、cost 与 evidence。

## 后果

- 垂直切片（vertical slice）开发更快。
- 事务一致性更易保证。
- 领域边界保持显式，后续如有需要可将模块抽取为独立服务。
- POC 阶段不得仅出于架构审美而做微服务拆分。
