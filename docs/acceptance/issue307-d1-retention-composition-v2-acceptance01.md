# #307 D1 V2 完整维护入口：非删除验收矩阵

本轮保留已发布 `f4e25a88f62da6bb14ed24da8b71b634be43f17a` 为基线；只在独立本地 checkout 实现和验收。#332 锁定、Draft、不合并；#307 OPEN。新 P 的完整绑定见提交后另存的受限发布提案；旧发布授权不复用。

权威有限契约：[retention-composition-v2](issue307-d1-retention-composition-v2.md)。两份固定 GOV 版本及全部旧数据、预算、pins、失败事实不变；不分配真实维护窗口/执行/预算，不生成操作授权。

| 项目 | 结果 | 证据与界限 |
| --- | --- | --- |
| 发布基线实时保护/CI/线程 | PASS_READONLY | `retention-composition-v2-acceptance01/retest03/published-boundary-readonly.json`；f4e25a88 的真实 PR CI 40082627590，13/13 success、同一 rule lockBranch=true、未解决线程0、Draft/Open/unmerged；不是新 P CI 或 main CI |
| 一致的有限 RetainUntil | PASS_OFFLINE | Python 准备/host、Go writer/reader/checker 同一绑定；支持 end+2h 与 end+72h 边界；拒绝跨字段错日、漏/null、fraction、超范围、旧 kind、重 hash 后扩对象/SDK 参数 |
| 完整顺序 | PASS_MODELED_COMPOSITION | before17 → business-races →可信关发/成功读回 → mixed → after17 →全闭合；独立 signed read/write 授权、原文 bytes、精确 P/保护/配置、预算与 one-shot run 入口 |
| 启动握手 | PASS_OFFLINE | Go 实际 false → O_EXCL/fsync ready → host 核 actual StartedAt/nonce/descriptor/config → fresh proof → 再核窗口/授权 → Enabled；缺 ready、重放、错身份、持久化失败和越窗均不启用 |
| 17 版本双 phase collector | PASS_MODELED_SDK_CORE_CHECKER_ENTRY | 原共享完整入口三组各 before/after 成功；before 接受两份精确旧期限、after 只改两行；其余 bytes/version/hold/Core/IAM/journal 不变；不是实际 native 保护写入验收 |
| 首份失败、UNKNOWN、预算与漂移 | PASS_OFFLINE | 禁止第二份；保留新旧事实与已用 slot。旧预算/归属、非空新预算、额外挂载、坏 policy/config、SDK拒绝、读回/日志/关闭失败不修数据续跑 |
| 动态 reader 封存 | PASS_INDEPENDENT_REVIEW | 六个固定读 nonce，create 前签意图、start 前签 CID 登记。首评发现漏 reader；复评发现坏 intent 会遮挡有效登记；均已根治并增加完整反例。主包/intent 坏仍关闭独立有效登记 CID；缺登记/关闭不明须 UNCONFIRMED |
| Python 回归 | PASS_LOCAL | 九个相关套件 normal 与 -O 均通过；最终共享 collector/完整 composition 各套件共13项，由独立代理按最终四文件原字节再次核验 |
| 当前 P 三个 Go 命令包 | PASS_LOCAL | writer、reader、readonly-check；原 checker fixture 日期落在旧截止前导致的失败已修测试夹具并保留原失败，不降低 invariant |
| 实际 R4 归档 | PASS_LOCAL_CI_ENTRY | `0446fde5985b51de7da5863b45df3f1b47b6167c`，parent R3 `53f6f8a6e69c17fa6896a40293d323f6ee9d0958`；ZIP SHA `a60ddfeb8a81d4414bc56e1ddd05fa01ca7f2e0e9579a7d403783bdd318d115a`，6515425bytes/781成员；从该归档重建后 pilot filter + 五个命令包通过 |
| 隔离真实 Linux | PASS_NONDELETE | 新 network=none/UID10001/no-root/no-socket 测试容器 `a3713c4afecc9a1d6a9b77c7a902ce89ffdd15f1abf9e7f1b669689d4b0aae06`，退出0/PID0；实际目录 fsync、SDK loopback double 两组读回、wait、三份新测试卷真实 custody；两份原 native 的 State 前后逐值相同 |
| 实际 Windows UNC 控制写入 | PASS_NONDELETE | 仅新 synthetic probe volume；实际 `r+b`/flush/fsync/readback、旧 bytes 归档、Linux tar/stat uid10001:10001/mode0600/nlink1 不变；未触碰旧卷/预算 |
| 独立整体收口 | 见 final review | 原问题及复评不覆盖，最终报告独立另存并绑定源码 hash；PASS 限于源码与非删除准备，不授予源启动/写保护/删除 |
| 新 P 发布与精确 HEAD PR CI | NOT_AUTHORIZED / NOT_RUN | 需新受限发布授权；只允许既有 rule 的 lockBranch 有限维护、单次 FF、新 head 立即重新锁定，再验收该 HEAD 全 PR CI；其他保护不得改变 |
| 实际两份 retention 延期 | NOT_AUTHORIZED / NOT_RUN | 发布后才准备实际 packet/MaintenanceID/独立预算和完整确认方案；另需新的人类精确授权。R4 default binary 无 trusted host key，不能写 |
| 后继剩余六份 | NOT_AUTHORIZED / NOT_RUN | 本轮没有新 deletion candidate/execution/窗口；新期限不代表覆盖任意未来 RecoveryEnd，后继需重新 full17 + 当前保护 + 全绑定核验 |

## 保留与失败口径

`retention-composition-v2-acceptance01` 保留首次 collector test-double 缺 Docker Path/Args 的失败原 stdout/stderr、原运行脚本和命令清单；后续 retest02/03 各使用新 O_EXCL 路径。它们是测试夹具错误，未发生真实 Source/provider 活动，也未改门禁来“通过”。独立两次 P1 发现和最终修复同样追加留证。

所有真实 fixture Source/Core/native start、provider protection write、业务 DELETE、cleanup、budget reset、pin release、分支 unlock/push/merge 均为0。本轮真实 Docker 活动仅针对新隔离测试容器/三份 synthetic 卷；它们及其输出保留。历史业务/失败/UNKNOWN/成本不回写；旧预算/pins未修改。

## 下一步的有限闭环

先按绑定新 P/R4 的受限发布提案获得本代发布授权，发布后精确 P CI 通过并重新锁定。随后只准备完整的两份延期 packet、真实读/写同一原文引用与有限维护窗口供确认。完成实际延期后的 full17 读回，才可能准备剩余六份删除的新方案；本轮不提前申请或复用任何保护/删除授权。
