# #307 D1：bind 路径身份修复验收矩阵

2026-10-25。本轮只修复宿主对 Docker Desktop bind Source 的解释；[身份契约](issue307-d1-retention-bind-source-contract01.md) 是有限实现边界。基线实际 M 为 `44c9bce432a5b5e393f3587964391e928a2639cc`，修复位于独立 `d1-mountfix` 工作目录，未推送。原 `d1-m44`、两个未启动 writer、五份卷、新旧预算、pins 和原失败保留；#307 OPEN。

| 验收项 | 事实与结果 |
| --- | --- |
| 根因及实现 | retention 原来绕过已有 `docker_host_paths.normalize_source`，把 daemon 的 Linux Source 交给 Windows Path。现在复用 namespace 转换，再以原组件检查、两侧 regular/nlink1、OS samefile 验证物理身份。另一份相同内容文件不能代替批准文件 |
| 文件身份拒绝 | 非文本/空/NUL、相对/drive-relative、错误 drive、混用 Desktop 斜线、`.`/`..`、空分量、未知别名、UNC/device、ADS、尾随点/空格、缺失/目录、同内容不同文件、hardlink、symlink、junction 均由相应平台反例覆盖；Linux 不借用 Windows Desktop namespace |
| 允许的原生表示 | Windows native、Desktop drive、大/小写、8.3 表示只有实际同一未链接文件才接受；Linux native absolute regular file 成功。不是宽泛字符串替换，也不以 content hash 代替文件身份 |
| 完整维护模型成功 | 新增实际 Windows Desktop Source 的双 owner 完整模型，真实调用 host.attest/topology，经过业务顺序、物化、ready、签名、关闭、before/after 聚合模型；两个 owner 正常完成。Docker/SDK 为 doubles，不是真实源验收 |
| rehashed 挂载反例 | 先重算 owner descriptor 和 bindings，再验证同内容副本、hardlink、extra/duplicate mount、writable binary、wrong type、额外共享预算 owner 仍拒绝；避免只因旧 hash 不匹配而假通过 |
| 原门禁保留 | 完整 raw Docker descriptor、固定四项挂载、类型/RW、volume identity、完整挂载 CID 集合、binary/config SHA、UID/security、Running/ready/proof 约束均未放宽。`retention_composition.py`、`retention_staging.py`、`evidence_indexes.py` 均未修改 |
| Windows 非删除回归 | 实际 CPython **3.14.6**；10 suites、20 条有效 normal/-O 命令，148 次方法调用含 6 次显式 skip，全部有效命令 PASS；forbidden retained reads=0。本轮 Windows **3.13 未运行**，不引用旧版本结果冒充 |
| Linux 非删除回归 | 实际 CPython **3.13.16**；相同 10 suites、20 条有效 normal/-O 命令，148 次方法调用含 6 次显式 skip，全部有效命令 PASS；forbidden retained reads=0 |
| skip 与原件 | Windows 无 symlink 创建权限明确 skip；真实 Windows junction 和 Linux file/parent symlink 均实际创建后拒绝。Linux 的 Windows-only Desktop/junction/完整模型按平台显式 skip。首轮 40 条命令已全部 PASS；将原 silent-continue 改为显式 skip 后，只复测受影响 suite 的四条命令，合计实际 44 条命令。原初轮及复测原件分别保留 |
| 实际冷态 bind 比较 | 对既有两个 `created`/PID0 writer 的真实 inspect Source 调用本轮生产 `bound_file_source`，均 samefile=true、nlink1、同原 R4 binary SHA、同原 cold descriptor，仍是固定四项 mount；**只证明 cold bind 文件身份**，不冒称完整 live topology、ready 或运行签名 |
| 冷态保全 | 前后九个 CID（原三组 Core/native、原 Redis、两个 writer）的 State/descriptor/RestartCount 相等；五份卷 metadata、Linux UID/mode/nlink/UNC 字节、完整 private 文件 SHA/bytes 和全局卷名相等。原 M cold 公开索引 12 成员及来源门禁字节不变 |
| 权限与预算 | 两份 permission 均 false、BindingsSHA256=null，output 为空，共享预算为空且没有 ownership.json；没有读取实际 host.private，也没有用它签发 proof 或 read/write grant |
| Linux 测试 helper | 仅两个离线临时 helper，network=none，只挂独立测试导出与输出，无保留资源或 source volume 挂载。复用既有 temporary_containers，先存完整 archive/日志再 ordinary rm；两份测试 helper 已移除。原两个 writer/五卷未动；volume/image/network 删除均 0 |
| CI 接线与发布状态 | 新文件身份 suite 的 normal/-O 已加入既有 Python CI collection；补丁尚未发布，因此本补丁 PR CI/main CI 均 NOT_RUN。旧 M/main CI 仅是基线来源证明 |
| 独立终审 | `PASS_SOURCE_AND_EVIDENCE_REVIEW_WITH_DISCLOSED_READ_DEVIATION`，范围内 remaining_blockers=[]；重核 7 份源/测试/契约 SHA/bytes、40 条有效记录的 120 份日志/读取审计 SHA 及全部原 collection 的 44 条命令统计；未由评审者复跑测试或执行原生源验收 |

原失败仍以 [M 冷态验收](C:/Users/Henry/.codex/worktrees/d1-m44/docs/acceptance/issue307-d1-retention-cold01.md) 为原始事实：旧代码的两次 FileNotFoundError 不改成成功。新代码的实际冷态比较另存新回执，不覆写 old blocker、旧授权、已用预算或历史结果。

本轮实际 source/writer start=0、provider I/O=0、保护状态写入=0、业务对象删除=0、新候选=0、新窗口=0、ownership 初始化=0。测试仅使用临时模型密钥签名；没有操作现有维护私钥或创建实际授权。

独立评审存在已披露的读取偏差：代理额外打开两个离线测试 helper 的 archive ticket，读取了其原始 Config.Env，违反本轮“不读原始 Docker Env”的评审约束。未读取 retained/source Env、私钥或源凭据；没有写文件、复跑测试或调用 Docker/WSL/provider/GitHub。报告保留带披露的实际终态，不声称评审过程完全未读任何 Docker Env。该偏差不改写当前代码阻断数，也不被用作未来操作授权。

发布前不能运行本地补丁去借用旧 M 来源授权。未来最终绑定须核实际已发布 source-ref；BudgetID 由 MaintenanceID 和实际 source-ref 派生，不能把冷态预留旧 BudgetID 直接当作新来源的最终 ownership。现有 R4 binary、密钥、两个停止 writer 和五份卷可先保留，本轮没有重建它们或新分配执行资源。

后继真实条件仍未完成：新精确发布来源及其 CI/授权、最终期限/时间/BindingsSHA、预算 ownership、实际 read/write authority、完整 source before/after 与保护延期。剩余六份删除也仍 NOT_AUTHORIZED / NOT_RUN。本轮不准备候选、窗口或调度。

本轮证据将保存在 [retention-bind-source-fix01](evidence/issue307-d1/retention-bind-source-fix01/evidence-index.json)。原始测试输出、文件读取审计和 Docker 原始观察保存在私有 `retention-mount-fix01`，公开摘要仅提供必要事实和原件 SHA。
