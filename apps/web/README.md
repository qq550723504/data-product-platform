# POC Web Console

The web console is the operator-facing shell for the Enterprise Activity vertical slice. It consumes only Core Platform APIs; OpenMetadata, Apache Hop and Splink are deliberately not top-level navigation concepts.

## Configure

```bash
cp .env.example .env.local
```

Set:

```text
PLATFORM_API_BASE_URL=http://localhost:8080
POC_WORKSPACE_ID=<workspace uuid>
```

`POC_WORKSPACE_ID` selects the workspace demonstrated by the POC. Individual resource, Dataset, Execution and Product IDs are discovered through Core read models instead of being pasted into the UI.

## Run

```bash
npm install
npm run dev
```

The console is available at `http://localhost:3000` by default.

## POC navigation

- 工作台
- 数据资源
- 数据集 / 不可变 DatasetVersion
- 数据生产 / Execution trace
- 实体审核（read-only in UI 2/3; decisions are added in UI 3/3）
- 数据产品
- 证据中心 entry point

The next POC UI increment adds entity review actions, Product Release readiness/publishing, and Release → DatasetVersion → Execution → Evidence traceability.
