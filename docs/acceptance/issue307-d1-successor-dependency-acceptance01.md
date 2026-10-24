# #307 D1 后继依赖拆分验收矩阵

本轮基线为本地 `182946ca8a130e97da06418b50fbabef58ab90bb`，实现位于独立 `d1-successor-dependencies` 工作树。原 `d1-retirement`、旧候选、预算、pins、失败证据及原清理回执均保留。权威设计见 `issue307-d1-successor-dependency-contract.md`；原维护契约和过去验收结论不回写。

状态：`PASS_BOUNDED_SUCCESSOR_DEPENDENCY_SPLIT_NONDELETE_ACCEPTANCE`；本轮有效阻断为0。

## 实现边界

原维护复验显式命名为 `retirement_catalog.verify_maintenance_snapshot()`，仍比较维护期间整机保全快照。后继 `historical_gate()` 继续调用 `retirement_catalog.verify_history()`，但后者现在组合三个独立步骤：

1. 精确审计历史索引、执行源码、回执、PG归档、原件、预算和pins。
2. 从固定事实推导7个源角色、保留数据保管者和前向netns父，检查当前184个必要owner及134个保留卷；不把无关会话历史State带入集合hash。
3. 对全部58份旧预算读取当前物理字节，与原tar语义树比较；不重置、不退款。

稳定依赖hash为 `13a4e86c9fc2fed5ef73d80ed07d887f65a01f4dfcc894200cebfd23558fc59a`，覆盖完整descriptor、卷元数据与完整挂载关系。返回新的v2语义身份，不兼容旧allocation，不续用任何旧execution/授权；当前候选自身拓扑和发送资格仍由原有门禁单独控制。

## 验收矩阵

| 条目 | 结果 | 证据（相对于 evidence/issue307-d1/successor-dependencies01/） |
| --- | --- | --- |
| 有限设计独立评审 | PASS，0阻断；设计阶段与实现终审分开留证 | `independent-design-review01.json` |
| 完整成功及反例组合 | 新增14项完整CLI测试；真实临时证据，Docker/WSL为录制适配器；普通/-O均通过 | `regression04-normal.stderr`、`regression04-optimized.stderr` |
| 相邻回归 | 新14 + 原退役29 + 既有准备/完整入口31 = 普通/-O各74项PASS | `regression04.json` |
| 独立实现复评 | 独立普通/-O各74项PASS，0有效阻断；旧执行源快照实际只读核对通过，不重复全量扫描 | `independent-implementation-review01.json` |
| 必要集合推导 | PASS，7源角色/184必要owner/134保留卷；包括原live子集外的17个实际卷保管reader | `dependency-contract01.json`、`dependency-build01.json` |
| 现场当前必要依赖 | PASS，实际Docker只读检查184 owner、134卷及全部挂载集合；未启动源/worker | `actual-dependency-check01.json` |
| 固定完整非删除入口 | PASS；2026-10-24 05:15:40–05:21:00 UTC实际执行，54,310原件、986/106退役回执、184必要owner、134保留卷和58份当前物理预算均通过；执行期间5个源/测试文件字节未变 | `actual-complete01/result.json`、`actual-complete-process01.json`及原始stdout/stderr |
| 原维护证据保全 | PASS；原工作树仍为182946ca且clean；原182项及评审48项索引从原位置核对通过，未用新实现覆盖旧执行材料 | `original-preservation01.json` |
| GitHub边界 | P仍`e54967e822847e97316e703ee413ffc1298cb627`；精确PR CI `39983308178`、event=pull_request、13/13成功；#332 OPEN/Draft/unmerged/locked，#307 OPEN | `github-boundary01/result.json`及完整请求回执 |
| 新业务候选/预算/窗口/授权、17版本原生provider核验、保护写入及业务删除 | 本轮NOT_RUN / 未创建。旧授权不复用；PR CI不覆盖本地新源码，不是main CI | 本矩阵及最终收口记录 |

组合回归具体证明：无关会话停启/替换/新增及无关netns失效不会改变后继返回；必要服务新的关闭轮次可通过；同样变化仍会被原维护快照复验拒绝。必要源、保管owner、netns父缺失/运行/descriptor改变，保留卷丢失/归属变化、预算挂载扩张/内容改变/重置、旧tar或pins回写、回执缺失、退役资源重现、Docker/WSL错误及bytes/str混用均拒绝。

## 失败证据与保留

实现探针先后发现固定inspect原格式是CID映射，以及17个范围外真实保管reader，原失败分别保留为 `implementation-probe-failure01/02.json`；修正新读入与闭包推导，没有改旧事实或增加宽松格式fallback。首次组合测试发现新输出目录误用了必须已存在的路径验证器，随后发现测试夹具将应为绝对路径的旧卷引用写成相对路径；原始失败stdout/stderr保留为 `regression-failed01/02.*`，均未进入真实环境操作。修复后完整CLI成功路径通过。

本轮不清理更多容器/卷、不启动源、不改retention/hold/IAM、不释放pins、不创建新预算/候选/调度，不推送、不解锁或合并#332。完整D1仍未验收；旧两份GOVERNANCE过期等业务方案前置不因本切片通过而消失。

## 收口

设计与实现独立复评均PASS、剩余有效阻断0；本轮后继依赖耦合修复已完成。旧工作树、预算、pins、失败和清理终态不回写。新材料由同目录 `evidence-index01.json` 单独钉住，索引不包含自身及其校验回执。新源码仅本地提交，既有远端PR CI不代表该本地提交或main CI；业务候选、授权和执行均尚未准备或获准。
