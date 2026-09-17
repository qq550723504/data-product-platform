# CSV 接入到主体审核：第一条界面化切片

基于 #91 的演示与门禁基线，独立增量，不替代之前的发布/追溯验收。

## 操作流程

完整分支启动后打开 `/ingest`（演示环境为 `http://127.0.0.1:3180/ingest`）。
填写名称、来源说明、本次处理用途，选择自己的 CSV。预览在浏览器本地进行，
点击“登记来源并保存 RAW 版本”后服务端再次验证，再调用正式 Core HTTP 命令。
收到 SHA-256 验证通过的 RAW 版本后，显式确认基准来源角色并启动主体解析。
待审候选在已有 `/reviews` 队列处理。向导不执行 demo.mjs，也不自动审核或发布。

页面提供“下载 CSV 示例模板（合成数据）”，对应
`apps/web/public/templates/company-import-v1.csv`。示例含七列表头和两条合成记录，
信用代码有意留空；仅用于说明格式，不保证建立新主体或产生可确认匹配。
正式处理前应替换示例、核实来源与权限。模板下载和本地预览在只读模式下也可用，
不会创建 Core 业务对象。生产 standalone 构建必须带上 `public` 资源。

支持 UTF-8（含 BOM）、逗号分隔、双引号转义、LF/CRLF 和引号内换行。
上限 512 KiB、1000 条记录、64 列、每字段 4096 字符；超过上限应先拆分。
必填列 `source_company_id`、`company_name`，每条记录非空，来源键文件内唯一。
可选列包括 `unified_social_credit_code`、`legal_representative`、
`registered_address`、`entry_date`、`company_status`。
额外列会原样留存，但不因此参与当前匹配策略。预检不是语义质量或合规评估。
V1 不支持任意字段映射、Excel、其他编码、批量文件、通用行业解析或大文件队列。

当前只提供已有 COMPANY 规则模板 `park/matching/company-match-policy-v1.yaml`，
使用 ANCHOR 来源角色。这会允许 Core 按基准来源建立新主体；操作前需显式确认。
实际任务状态由 Core 返回。没有待审候选不代表全部正确，更不代表可以发布。
新接入的数据不会自动绑定现有演示产品或推进到生产/发布阶段。

## 数据与异常边界

原始文件字节不被字段映射、BOM 处理或换行转换覆盖。BOM 只在 Core 的
CSV 行数统计和主体解析读取边界跳过，存储内容和校验和保持原样。
资源 description 记录来源、用途、原文件名和接入标识；这些是操作声明，
不是正式 Authorization 或法律合规结论，不放行 Rights Gate。
服务端生成的存储文件名包含接入操作 UUID，避免把不同上传的同名文件当作同一来源。

入库通过已有 CreateResource、CreateDataset、UploadVersion、StartEntityMatchJob
命令。操作分为“保存 RAW”和“启动解析”，不是跨服务原子事务。
发生部分成功、超时或无法核实响应时，保留已知对象标识并锁定本次提交，
提示人工核对；不会自动重试、补造成功状态、回滚删除或覆盖已冻结版本。
相同接入标识的资源/数据集编码及相同 RAW 版本的解析输出编码用于阻止重复创建。
这不等于通用的持久幂等工作流：换新页面产生新接入标识，可能是新的独立导入。
解析输出容器创建后但任务结果不明，需要先诊断，不能靠反复点击恢复。

RAW 创建后有可保存的 `/ingest?version=<UUID>` 链接，数据集版本页也有
“继续主体解析”入口。页面刷新不删除历史。启动解析前服务端重新核对
工作区归属、RAW 类型、READY 状态、来源标识，并阻止为同一版本重复创建任务。

## 配置与访问边界

```dotenv
POC_ENABLE_INGEST_ACTIONS=false
POC_INGEST_ACTOR_ID=
```

默认关闭写入，仅允许本地预览。工作区与操作人从服务端配置取得，
忽略浏览器传入的 actor/workspace/policy/sourceRole。演示容器显式启用，
并使用 manifest 中的操作人。现有本机回环端口限制不变。
这些配置不是身份认证或租户授权；直接 Core API 的鉴权与多租户隔离仍需
独立实施。不要把本地演示端口改成公网后用于真实客户数据。

## 测试

`npm --prefix apps/web run test:ingest` 严格编译独立模块并执行 CSV/命令测试；
普通生产构建包含这些测试。覆盖原始字节、格式与限额、空理由/声明、
跨工作区与版本错配、服务端身份、分页、重复任务、部分成功和超时。
Go `csvinput` 测试覆盖带/不带 BOM、引号表头、空输入及非开头 BOM 保留。

原有 browser-contracts 增加默认只读接入预览用例。
真实服务套件在原有三阶段验收后单独执行 `TestBrowserCSVIngest`：
使用新工作区和动态 CSV，通过浏览器创建所有业务对象、保存 RAW、
启动解析并人工审核。Go 独立查询 PostgreSQL，再经真实 S3 客户端读取
MinIO 字节核对 BOM 和哈希，并核对审核人、理由、Evidence、Audit 与血缘。
该流程不通过 SQL 或准备命令预造资源、数据集、匹配任务及审核结果。

证据文件：`ingest-verification.json`、`ingest-live.log`、`browser-ingest.log`、
`report-ingest.json`、`html-ingest/` 和 `results-ingest/` 截图/追踪。
这些是合成数据验收，不代表真实身份提供方、生产权限、大文件性能或商业上线验收。

## 初次试用与版本选择

当前切片位于 `codex/csv-ingestion-review-ui`，基于 #91；是否合入 main 以 PR 实际状态为准。
从完整仓库根目录运行：

```sh
git fetch origin
git switch codex/csv-ingestion-review-ui
node deploy/demo/demo.mjs doctor
node deploy/demo/demo.mjs up
```

打开 `http://127.0.0.1:3180/ingest`。在自己的合成 CSV 上完成：下载模板并修改 →
预检 → 保存 RAW → 保存继续入口 → 显式启动主体解析 → 查看人工审核/输出版本。
不要运行 `advance` 来推进刚上传的 CSV：该命令仍只服务预置的经营活跃度演示样本。
审核队列按工作区显示全部候选；可能同时含演示样本任务，请核对向导返回的任务 ID。

`node deploy/demo/demo.mjs down` 停止容器但保留历史。
`node deploy/demo/demo.mjs verify` 验证的是预置演示产品，**不是新接入 CSV 的通用验收命令**。
新 CSV 的自动化集成验收由 `TestBrowserCSVIngest` 单独执行。
遇到部分成功先保存已知 ID 并核对，不要清空数据或反复提交来掩盖异常。
