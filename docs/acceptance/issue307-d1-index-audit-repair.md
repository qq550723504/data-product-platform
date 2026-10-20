# #307 D1 索引审计修复与有限验收

Goal：把 v26 窗口开始时漏在固定入口外的附件索引审计纳入 `verify-six-native.py`。独立索引的 `files` 映射与授权/调度索引的 `members` 列表有不同 contract，必须显式声明，不能猜格式或在窗口临时拼脚本。

本切片在 `d1-index-audit` 隔离工作树修改当前源码。原 `d1-cli-fix` 的 v26 HEAD、冻结源、候选、授权、失败终态及私有 seal-history 保留。v26 六份均 NOT_RUN，DELETE=0，原整体封存仍 UNCONFIRMED；本轮没有重新启动原环境、访问原预算或启用身份。#332 保持 Draft、不合并，#307 OPEN。旧授权不可复用。

## 唯一 contract 与复用边界

`examples/lifecycle-d1/evidence_indexes.py` 定义有限索引读取，完整入口负责调用顺序，现有 `collect`/Go checker/finally-close 继续负责 Core、native、IAM 与技术请求事实。本次复用仓库的只读入口、Python 标准库 json/hashlib/pathlib 和 unittest，没有新框架、queue、policy engine 或基础设施。JSON Schema 可校验静态类型，但不能独自完成仓库根路径、链接拒绝、原字节 SHA/长度与当前候选绑定；这部分是最小的证据语义检查。

两种格式独立拒绝另一种容器；不接受 `auto`、旧 members-map 或任意格式回退：

| format | 索引容器 | 成员根与内容 |
| --- | --- | --- |
| files-map-v1 | `files: {relative_path: {sha256, bytes}}`，正整数 `count` | `member_root` 指定相对仓库的目录；例如独立报告目录 |
| members-list-v1 | `members: [{path, sha256, bytes}]` | `member_root=.` 时成员 path 相对仓库；例如授权与调度证据 |

共同约束：固定 plan SHA、固定 index SHA、期望 status 与正整数 expected_count；SHA 为小写64位hex，bytes 为非负 int、拒绝 bool。重复 JSON key、重复或大小写别名成员、空集合、混合容器、跨格式、未知/缺字段、SHA/长度不符、绝对路径、驱动器/反斜杠/点段/越界、symlink/junction、自引用均拒绝。不递归猜测子索引；批准 plan 必须逐项列出应验索引，所有条目均核验。

plan 的唯一 v1 结构为 `version / execution / candidate_sha256 / indexes`。每个 index specification 必须且只能包含 `id / index / sha256 / format / member_root / expected_count / expected_status`。plan 原字节 SHA 由调用参数绑定，execution 与 candidate SHA 再与实际 candidate 原字节比较。未来新方案须在独立评审和完整确认中冻结 plan 原字节及全部索引清单；准备阶段尚不存在的未来授权/调度索引不能伪造为已有证明，获得真实授权后另存并核对当前实际 plan。

## 完整入口顺序

1. 校验 candidate、bindings、Windows file-only checker 原字节绑定及新输出目录。
2. 必须提供 `--evidence-audit-plan` 与 `--evidence-audit-plan-sha256`；固定入口完成显式格式与成员审计。旧参数缺省不能静默跳过。
3. `--audit-only` 只输出 `PASS_SAVED_EVIDENCE_AUDIT_ONLY` 和 evidence-audit.json，标记 `native_preflight=NOT_RUN / execution_authority=false`；不会进入 host gate、Docker、Core/native 或 provider。允许只读检查旧终态证据，不允许恢复旧 execution。
4. 非 audit-only 路径先拒绝已存在、别名或不可读的 execution terminal；然后才是原 sealed case/topology/budget 门禁。
5. 通过后使用原三组 actual-collect/Go checker/finally-close；complete.json 同时引用 evidence_audit。中间组失败保留原错误/关闭回执，后续组和 complete PASS 均不产生。

索引审计成功、模拟完整 main 成功、真实 saved-evidence CLI 成功、实际 native 前置成功、精确 HEAD CI、独立评审和人类执行授权是不同事实。该入口没有 enable/DELETE 功能。

## 非删除验收矩阵

| 验收项 | 本轮事实 |
| --- | --- |
| 原失败保留 | v26 window01 原失败、103步seal、445 CID关闭读回、30false/6case/6recovery撤销、终态及已暂停调度原字节复制留证，原工作树不修改 |
| 实际历史文件审计 | 固定 main 的 audit-only 成功检查独立53、授权19、调度12，共84条成员引用；只是 saved evidence，无 native 或执行授权 |
| 完整成功组合 | 真正 main → 真正 typed audit → 三次真正 collect → finally → complete；Docker/PG/SDK/checker 是受控 doubles，非原生验收 |
| 失败组合 | 缺参数、plan SHA、错格式、成员漂移、旧terminal、host拒绝、已有输出、第二组checker失败；启动前拒绝或全部关闭，第三组不跑 |
| 格式与成员反例 | 两种格式/排序绑定、混合/交换/未知格式、重复JSON/成员、自引用、空/少成员、status/count/bytes/SHA类型与值、路径越界及链接拒绝 |
| 当前自动回归 | normal 与 -O 各50 methods，49 PASS / 1明确SKIP；Windows账户不允许实际symlink创建，另有不跳过的junction边界反例；Linux CI覆盖实际symlink |
| 固定入口旧终态拒绝 | 实际 CLI 已按预期拒绝旧 terminal，没有创建输出目录或进入host/Source；不可读terminal也按回归拒绝 |
| 本轮独立评审 | PENDING，未将作者测试或旧候选评审冒称新修复复评 |
| 当前修复HEAD对应PR CI | PENDING；旧74aa的绿色不是新修复CI |
| 新候选/新execution/新窗口 | 独立复评通过后再准备；当前未冻结、未授权、未创建调度 |
| 原生17版本fresh核验/发送/恢复 | 本轮NOT_RUN；旧v26不能复活，后续新方案分别验收 |
| 完整D1 | NOT_ACCEPTED |

Merge blockers 仅限本切片 contract 不一致、成功必需路径不可达、索引/候选绑定可绕过、原事实被覆盖、失败后仍有副作用、回归或 CI 失败。新业务语义、迁移、IAM扩权、删除范围/窗口扩大均不在本切片。有效阻断横向修复一次，独立复评通过后才准备后续方案，不把准备 PASS 当作授权。
