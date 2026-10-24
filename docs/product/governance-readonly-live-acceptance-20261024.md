# 默认只读预览：独立库现场验收

日期：2026-10-24。前置提交546f39bc；本批精确提交与CI终态记录在PR363 closure comment，不以旧HEAD绿代替新HEAD。

## 授权及实际范围

用户明确要求：预览默认只读，用独立验收库补当前DDL/最小权限并完成只读API与浏览器验收，保留原库和Agent API，不接模型或派发真实执行。本次不合并或部署。

唯一原owner checkout为aq1。保留既有loopback25494 PostgreSQL容器，未停止tmpfs数据库。新库agent_q2_acceptance_20261024，不重建/迁移/seed原agent_q2_test。专用准备工具复用Migration Runner、Q1合成fixture与Evidence.CreateSnapshot；合成发布行仅为读取夹具，不是发布Command成功或真实交付。没有启动worker/引擎/provider/模型。

## 默认关闭与最小权限

- 专用host取消启动bootstrap，要求显式独立库DSN并拒绝原库。AGENT_TASKS_WRITE_ENABLED默认false；关闭时连接default_transaction_read_only=on。生产Agent API实现保留。
- start.ps1强制API与UI/BFF写开关false；Caddy保留认证并拒绝非GET/HEAD。GET路径与原principal/workspace规则保留。
- 新reader只获显式读取表清单SELECT；有效表写权限0、列写权限0、schema CREATE=false、TEMP=false、public函数EXECUTE=0，角色与连接默认READ ONLY。未授planner所需的fence/copy锁写权限；只读验收不需要它们。
- 新角色随机口令只保存在gitignored本机profile，Windows ACL仅当前用户与SYSTEM；不在文档/日志输出。准备脚本拒绝已有库/角色，不自动清理/重置。

## 实际DDL与API

当前完整migration安装在新库，两个aa_lifecycle_ref触发器分别位于agent_plan_revision.source_version_id和agent_plan_input.dataset_version_id，均enabled=O。没有用schema73数字代替实际trigger核验。原库缺口保留，未补写原库DDL或权限。

现场API四组测试PASS：Agent authority/list、context、quality history/report、certification history、release history GET；明确Trace的冻结快照、integrity及安全投影；401/403/404/400边界；直接API和BFF的POST403、gateway非读取方法405。命令探测在业务Service前拒绝，未产生业务写入。

浏览器暴露并修复一个实际生产投影缺陷：无mapping时appendUniqueEntityMappings把空集合变成nil，JSON为null，使严格Trace validator判UNKNOWN。修复合并函数返回非nil空数组，保留按immutable decision去重；增加Go序列化/决策身份用例，现场API显式断言空数组。未放宽前端校验、未伪造mapping。

## Edge浏览器验收

直接只读入口4188，gateway4189另由API验证认证与方法限制。按UI操作完成：发布历史PUBLISHED筛选→精确发布Trace→展开冻结成员/数据版本→明确版本质量→historyOffset25的历史评测→刷新不换报告→findingOffset25的第26条发现→认证空状态→浏览器返回保留报告与页码。Agent工作台保存按钮禁用。缺失发布显示UNKNOWN、原发布标题消失、不回退latest；恢复明确发布后返回历史仍保留PUBLISHED筛选。

合成发布30303030-3030-4030-8030-303030303030，快照4af32c20-68eb-41ab-9309-628ae7df89c7，根哈希ebcb557c34a7cfc5c036a016418ab67924f7b2c17c86932927385f61c9072f84，页面INTEGRITY_OK。没有执行/映射的集合正确显示0，不推断生产完成或成本为零。此为本批路径验收，不扩展为全站视觉/无障碍认证。

## 无写入与保留证据

准备与读取窗口分开。对原库、新库全部216张public表的count+内容摘要按表名比较，读取窗口后均无差异。初始列表顺序比较曾因聚合返回顺序不同呈false；逐表映射核对差异为0，fingerprint函数已排序以避免误报。

新库Execution=0、AgentTask=0、ActionRequest=0、Outbox=0、synthetic ProductRelease=1。原库未复制数据至新库。API/Next/gateway仅本轮归属进程，原数据库容器不停止。默认只读预览保留供用户查看。

本机证据（gitignored）：governance-readonly-build.log、governance-readonly-api-final.log（4/4）、governance-readonly-no-write-proof.json、governance-readonly-trace-edge.png及受限runtime profile/owner记录。原库及新库fingerprint基线无业务行原文。Go离线默认只读/事务前拒绝、Trace空集合回归通过。最终新增提交需独立增量审查与精确HEAD完整CI；旧独立审查报告不会自动覆盖新文件。
