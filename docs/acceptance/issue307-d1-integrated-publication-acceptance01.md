# #307 D1 本地整合非删除验收矩阵

状态：`PASS_BOUNDED_LOCAL_INTEGRATION_NONDELETE_ACCEPTANCE`，当前切片有效阻断0。源码整合提交为 `b2ea34cfab49d445b27d239278cb5a4f9fbc053a`，第一父e54967e8、第二父be38a68d。随后仅提交本矩阵、只读验收/独立评审回执及精确文件的字节保全属性；最终发布HEAD另由受限发布方案钉住，不改运行源码或R。

| 验收项 | 实际结果与边界 |
| --- | --- |
| 祖先/历史保全 | 双父关系精确，e549全部37,151路径均保留；34,090个已发布#307证据Git对象不变。be38新增323路径全部保留，其中successor测试额外增加2项整合回归 |
| 唯一冲突 | prepare当前history钩子唯一调用catalog v2；旧C23包装函数重命名用于历史解释，旧pre-C22函数保留，无错误fallback；其余AST等于e549 |
| 已发布修复与R | retention Go/宿主修复、C23 profile/窗口/网络不回退；runtime-spec blob仍aa009fbd15440e5e00769995d093088f0edd9022，R仍ecf40e6c7a9b3aa11166e6a56a5163a29a15691d |
| 本地CI配方非删除回归 | 现有25个Python测试文件，normal/-O各177项：各176 PASS、1项原有条件skip（普通symlink权限）；Windows实际junction拒绝仍PASS。不是GitHub Actions/Python3.13的远端结果 |
| 保留R完整归档与离线Go | 原6479934 bytes ZIP SHA 5e86e36808f00f62b9e9c7eb9b6b76d0c306958f4c5118c6d9d4eb4ff00de092完全匹配；原固定CI入口重建并运行pilot及5个cmd包测试通过。仅临时目录，无源/provider操作 |
| 现场完整只读入口 | 2026-10-24T05:56:29Z PASS：54,310原件、986容器/106卷历史退役事实、184必要owner、134保留卷、7源角色和58份物理预算；稳定依赖hash仍13a4e86c9fc2fed5ef73d80ed07d887f65a01f4dfcc894200cebfd23558fc59a。没有再次退役或启动源 |
| 独立整合评审 | 当前实际源码b2ea34cf独立评审PASS，0阻断；normal/-O各100项PASS。实际关键raw SHA逐项与报告读回一致，未把be38旧评审当本轮评审 |
| 分支与当前已发布CI | 2026-10-24T05:57:02Z实时读回：远端仍e549，原rule锁定、其他完整保护不变，#332 Draft/open/unmerged，线程0，#307 OPEN；e549 run39983308178/attempt1/pull_request 13项成功，仅覆盖旧已发布HEAD |
| 最终新HEAD远端CI/main CI | NOT_RUN；没有推送或合并，不能以本地回归或旧绿色替代 |
| 权限与后续 | 解锁/推送授权NOT_RECEIVED，旧授权不复用。下一步仅呈现绑定最终HEAD的受限发布方案；新候选/execution/预算/窗口、保护延期和业务删除均未准备或执行 |

原被阻断的be38直发方案和其54项证据复制原字节保留；原数据、pins、预算、失败/UNKNOWN/成本事实不回写。新回执位于 `evidence/issue307-d1/integrated-publication01/`，索引排除自身与自身审计，避免自引用。本切片通过不表示完整D1已验收。
