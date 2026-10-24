# #307 D1 candidate22/v32 有限源码与准备边界

本文件补充[01准备意图](issue307-d1-v32-preparation-contract.md)，01及其索引保留为维护许可前快照。新execution为`0e5cb8f3-46a9-4339-88e4-7154d6ebe13f`，人类选择的窗口为2026-10-23新加坡18:00–18:40（UTC10:00–10:40），只读恢复最晚12:40Z。删除及调度均未授权；#332保持Draft不合并，#307保持OPEN。

## Goal、权威和范围

只准备P2→T1→T2→H2A→M1A→M1B六份exact目标，每份max1、全局max6，UNKNOWN不重发；P1永久排除。十一份survivor、所有环境、备份、旧预算、pins及失败/UNKNOWN/成本事实保留；cleanup=0、hold-release=0、bypass=0。完整D1仍未验收。

执行与分支维护以[execution冻结边界](issue307-d1-execution-freeze-boundary.md)为权威；Go编译scope、remaining_window/remaining_six_window与现有固定完整核验/准备入口拥有时间、目标和核验语义。Git/GitHub原生保护、官方MinIO SDK、既有Outbox→Asynq、既有typed evidence/path/owner/budget核验继续复用，不新增平行队列、工作流或policy engine。

本地独立分支允许准备源码、执行非删除回归并留下新记录；它不推进旧锁定PR HEAD。远端#332更新须在新鲜保护核验及已获的人类维护授权下受控解锁后进行。源运行、构建分配及下一阶段验收仍各自遵守下述门禁，不能由本地回归、旧CI或维护许可替代。

## 本轮有限增量

- Runtime R为`81a083751c01a281d42df594f6e91012255081f1`，直接派生自原R `514843da1a143fba537c1668483fe95800ddd992`，只修改5个fixture源码/测试成员。保持真实schema69、66个migration成员、无protected identity；不迁移或重建三源，不增加产品legacy fallback。P保留现有产品模型，独立绑定源码、原始raw/LF与后续精确source CI。
- 更新新execution/window/身份命名；将candidate21实际创建的6个旧身份加入完整审计，保留其exact-version双action策略。candidate20的未创建身份仍以fresh NoSuchUser核验，旧Version-only策略不改写。
- 用既有typed index、路径/资源/owner和budget函数保留过期candidate21。原public/private终态SHA为`5568708e62c4ad37f8765aba667697de547d83fe0e598d59d7d4341a84b4b188`，原2257项私有文件、6061项公开事实/回执、86个分配owner和3个技术reader保留；原历史链907 CID之后完整集合为996 CID，旧预算30个、实际备份66份。新代资源尚未分配，未来须有3个独立预算，初始为空且归属/完整挂载集精确。
- 分配前先复用严格host_snapshot检查已提交源码与未跟踪宿主代码，再核其精确P HEAD的PR CI；dirty源码不能借用旧HEAD绿灯进入历史读取/构建。现有构建后源码再核对及首次资源创建前门禁保留。
- 一份旧v31准备文档在新checkout被CRLF转换。原失败、原新checkout字节及SHA已保留；确认仅LF等价后恢复原issued字节，并增加该文件的精确`-text`规则及Git blob/archive/两种checkout回归。旧发行原件不修改，不以文本等价放宽原始SHA核验。

## 分支维护许可

人类已直接授权：实时核对HEAD、ruleID和完整保护一致后，仅解除#332的分支冻结位，继续准备candidate22/v32，最终CI通过后重新锁定。原HEAD为`f4f27c789f53b6c45910f55f83bee6cabfdee4f6`，rule为`BPR_kwDOUdQLBs4FE_7b`；许可只允许发送`branchProtectionRuleId`和`lockBranch=false`两字段。管理员约束、fork-sync/force/delete限制及全部其他保护保留；不删除rule，不自动重试unknown mutation，不声称服务端CAS。

原始TLS/connector 403及执行前重读失败留证。新鲜完整保护和宿主闭合未通过则不发送维护mutation；只读检查PASS不冒充解锁成功。维护与删除/调度授权分开，旧删除授权不复用。最终所有源码/候选/验收/评审/静态文档完成并获得最终精确HEAD CI后重新原生锁定，再写新的execution-binding。锁后继续仅追加回执，不能再更新HEAD。

## 完成顺序与merge contract

源码独立评审及其精确P HEAD全部必要PR CI成功 → 新allocation/独立预算 → 三组真实非删除source与隔离restore/rehearsal验收 → 完整17版本/库存/保护/IAM/Core只读核验 → 全绑定冻结与独立评审 → 最终HEAD精确CI → 原生重新锁定及execution-binding → 完整精确确认文本。之后仍需人类新删除授权；定时续跑另按人类指令建立。

仅安全/授权或凭据泄漏、历史/预算/pins/成本改写丢失、权威contract冲突、必需路径/CI/build/test失败以及并发/幂等破坏当前invariant构成阻断。新产品生命周期、源schema升级、额外provider、一般网络/观测hardening与其他环境不扩入本切片。NOT_RUN、NOT_AUTHORIZED、SKIP、UNKNOWN与PASS分别报告，副本/模拟不能冒充源、native或provider事实。
