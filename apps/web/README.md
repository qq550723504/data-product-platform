# POC Web 控制台

Web 控制台是面向运维人员的企业活跃度垂直切片外壳。它只消费 Core Platform API；OpenMetadata、Apache Hop 与 Splink 被刻意设计为非顶层导航概念。

## 配置

```bash
cp .env.example .env.local
```

设置：

```text
PLATFORM_API_BASE_URL=http://localhost:8080
POC_WORKSPACE_ID=<workspace uuid>
```

`POC_WORKSPACE_ID` 选择 POC 所演示的 workspace。具体的 resource、Dataset、Execution 与 Product ID 通过 Core 读模型发现，而不需要在 UI 中手动粘贴。

## 运行

```bash
npm install
npm run dev
```

控制台默认可通过 `http://localhost:3000` 访问。

## POC 导航

- 工作台
- 数据资源
- 数据集 / 不可变 DatasetVersion
- 数据生产 / Execution 追溯
- 实体审核（UI 2/3 中为只读；决策在 UI 3/3 中加入）
- 数据产品
- 证据中心入口

下一个 POC UI 增量将加入实体复核动作、Product Release 就绪/发布，以及 Release → DatasetVersion → Execution → Evidence 的可追溯性。
