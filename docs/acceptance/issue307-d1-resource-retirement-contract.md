# #307 历史验收资源退役边界

本切片只维护 Docker 测试资源，不授予 provider/object DELETE、重新启用身份、调度、合并或部署权限。candidate22/v32 的分支、源码、execution-binding 和原始材料保持原样；其 2026-10-23 18:00–18:40 新加坡窗口已过，未获本代删除授权，不得续用。后继业务方案必须重新冻结、重新授权。

## 根因与复用

旧 `historical_gate()` 把历史证据、预算事实和 Docker 对象存在性混成一个递归实时断言；`allocation_state()` 又把这个断言与旧分配快照逐字段比较。历史关闭证明因此成为永久基础设施依赖。

复用 Docker 原生 inspect/logs/cp/rm、Python 标准库和仓库既有 `evidence_indexes` 路径、类型与 SHA256 校验。没有新增守护进程、workflow、清理框架或 Core 领域实体。Docker 原生 prune 无法表达本切片的精确 CID、预算和不可变失败证据边界，因此只增加有限退役契约。

## 有限不变量

1. 原审计的 986 个完整 CID 只是候选上限。必须重新核对现场、归属、原绑定及完整依赖。源 PostgreSQL/MinIO/原业务 Redis 共 7 个、candidate22 的 86 个、6 个共享 netns 根以及其 139 个集合外引用者均保留；后续 candidate23 和维护资源按新鲜归属另行保留。首阶段只删除容器；经 2026-10-24 新的人类清理指令，第二阶段另逐卷绑定空测试卷或有完整冻结备份的旧 PG 恢复副本。网络、镜像均不删除。
2. 归档完整 inspect、带时间戳原始 stdout/stderr、原权限/closure/失败/预算档案及其 SHA256。原始私有材料留在原处并逐文件钉住；预算通过停止的原 wire 只读 cp，归档其原始 tar。归档不变成“当前关闭状态”。
3. 固定计划明确区分 `live_required` 与 `retired_historical`。准备检查要求候选仍存在且关闭；退役检查要求固定归档、逐项追加删除回执以及成功完整 ps 清单中旧 CID 确实不存在。未列入计划的 missing CID、Docker 错误和归档失败均阻断。
4. 真正保留的资源、预算卷元数据及剩余挂载集合继续现场核验。旧预算不归零、不退款，UNKNOWN/失败不改写，原 manifest、pins 和备份不修改。
5. 先完成固定入口、成功/反例回归、收口审查和维护绑定冻结，再允许删除。每批先刷新全机 netns 反向引用；只按完整 CID 有序 `docker rm`，不用 force、volumes、prune。外部引用、重启、权限或挂载漂移即停，原失败留证。
6. 不把旧 current-allocation 快照改成新契约。仅后继源码的 `historical_gate()` 可使用显式编译绑定的退役 catalog；未完成退役时不能冒充 PASS。后继 allocation 自行冻结新的历史契约结果，不能继续运行原 C22 execution。

## 已发现的原始表示漂移

未适配的归档首次检查在 v27 完整配置哈希处失败，原失败保留。620 个原 descriptor 中，543 个现场对象的 `HostConfig.Dns=[]`，而原始完整哈希使用 `null`。逐 CID 仅将这一字段表示为 `null` 后，全部 543 个完整 Image/Config/HostConfig/Mounts 哈希精确匹配原值；无其他字段差异。原哈希和实际完整 inspect 均不修改。

维护归档比较显式复用这一有限语义：只对已由原 v27 plan SHA 钉住的 CID 和完整配置哈希，允许无自定义 DNS 的 `null/[]` 表示差异；新增 DNS、权限、挂载、网络或任何其他配置变化仍拒绝。适配只存在于隔离维护比较的只读进程边界，实际归档保留未经投影的 inspect，当前 State 仍来自现场。它不产生原 frozen C22 门禁 PASS，不用于旧 execution，也不修改旧源码。Docker 的 resolver 源码仅在非空 dnsList 时采用自定义服务器；参考 [Moby 原生 DNS 配置实现](https://github.com/moby/moby/blob/master/daemon/libnetwork/sandbox_dns_unix.go)。导致本次表示变化的外部操作仍为 UNDETERMINED，不把推断写成事实。

## 完成口径

分别报告已归档、已删除、仍保留和未完成项。没有逐卷验收前，197 个独占卷线索仍为 NOT_APPROVED。容器数减少不等于回收镜像或卷的相同空间。#332 保持 Draft/unmerged，#307 保持 OPEN；本维护过程业务 provider 调用、对象 DELETE 均为 0。

## 预算集合根因与当前边界（2026-10-24 追加）

capture04 的 `FAILED_NO_REMOVAL` 原件保留，不改写。原代码将预算数硬编码为 33，遗漏原范围内 7 个独立 host-path 演练预算；之后 candidate23 又增加 3 个新预算。当前完整宿主挂载图观察到 58 个不同预算卷：候选07–23共51个、独立演练共7个。旧33、原范围40、当前范围43、全图58是不同集合，不互相冒充。

新入口按原 packet / allocation / 失败演练 terminal 的精确卷名、execution、allocation nonce、wire CID 和完整挂载CID集合取并集；等价重复引用去重，冲突、遗漏、新挂载或归属改变拒绝。58个预算全部保留原卷与原始tar，used/UNKNOWN不退款、不重置；删除旧挂载者后，后继门禁仅允许精确追加删除回执对应的挂载差集。

逐卷计划覆盖全部197个候选。预算无条件保留；空卷须有停止原owner的离线完整内容读取，任何非空文件、链接或特殊节点均不能当空卷。旧PG恢复副本须同时证明原分配角色、原候选packet与仍存在的core.dump/globals.sql/native.tar三项SHA完整一致。非空恢复队列或不能证明完整重建的独特副本继续逐项保留并列明原因。仅执行精确 `docker volume rm <name>`，执行前刷新全机反向引用及卷元数据，Docker自身原子拒绝仍被引用的卷；不使用force、prune或down -v。

后继catalog先核原归档/原始材料与精确容器删除回执，再核新增卷计划和卷删除回执，最后以成功的当前完整ps/volume ls与保留资源inspect证明缺失/保留状态。旧C22/C23冻结结果、HEAD、执行绑定和所有旧授权不修改；本地退役源不构成未来业务执行授权。
