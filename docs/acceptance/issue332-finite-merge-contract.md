# #332 有限合并收口契约

本轮基线为已发布 `1d142fa332b639144c232bb686be214521b9f352`。只修复后继依赖核验对 `HostConfig.Dns=null/[]` 的表示差异，完成非删除验收、独立终审与PR描述收敛。#332仍锁定、Draft、不合并；#307 OPEN。没有分支解锁、推送、源启动、保护写入或删除授权，不新建候选、execution、预算、窗口或调度。

## 权威边界与复用

领域规则仍由 `docs/architecture/data-lifecycle.md` 和 #307 有限契约拥有。资源退役历史由 `retirement_catalog.py` 固定原归档/回执，必要后继依赖由 `successor_dependencies.py` 拥有，原始 descriptor 由 `retired_resources.py::descriptor` 拥有。本轮不改其原始算法或任何旧快照/授权/失败。复用现有 descriptor、Docker CLI、标准库及已有证据校验，不新增策略引擎、容器框架或兼容产品模型。

Moby上游DNS实现仅在dnsList长度大于0时应用自定义nameserver，DnsSearch和DnsOptions各自独立处理：[原生实现](https://github.com/moby/moby/blob/master/daemon/libnetwork/sandbox_dns_unix.go)。此参考不声称上游master就是现场Docker构建，也不判断发生表示变化的外部原因。

## 当前必需不变量

1. 内层后继契约升级为 `d1-successor-cold-dependencies-v2`，比较算法钉为 `RAW_AND_EMPTY_DNS_EQUIVALENCE_V1`；旧v1不按新语义静默解释。外层稳定history字段不变，新的dependency_contract_sha显式变化，不复用旧冻结执行绑定。
2. build仍先核原始冻结行与历史raw descriptor完全匹配，保留每CID原descriptor。只对实际存在、类型正确且值为null或空list的Dns追加canonical-empty hash；原有非空DNS只允许严格raw相等。
3. verify要求算法标识及canonical hash一致，还要求当前行只改变Dns为null或[]的两个raw候选之一匹配历史原hash。新增hash不能独立覆盖原hash；不泛化删除所有空字段，不排序/忽略其他配置。
4. 缺字段、错误类型、增加nameserver、DnsSearch/DnsOptions、identity/image/command/env/user、权限、挂载、netns、运行/PID变化继续拒绝；原关闭状态、完整卷挂载图、预算归属/原字节、pins及历史回执检查不变。
5. 完整成功与反例在normal/-O及Windows/Linux Python3.13验收；真实宿主复核仅Docker inspect/ps/volume inspect、既有WSL只读预算读取，不启动源或调用业务provider。不能把mocked前后17版本流程当真实native读写。

## 合并阻断与退出条件

仅安全/授权或泄漏、历史/账务破坏、权威契约冲突、当前必需路径或CI失败、核心并发/幂等破坏属于代码阻断。针对本轮改动完成独立终审并列明BLOCKER/FOLLOW-UP/OUT-OF-SCOPE，不继续扩大候选或现场操作范围。

本地修复/回归通过不等于远端代码已更新。若本地修复尚未发布，则当前PR的原DNS问题仍是交付阻断；必须另获绑定新HEAD的受限发布授权，重新锁定并通过新HEAD完整PR CI后才具备合并候选资格。Ready/合并及实际merge SHA的main CI是后续独立授权和验收。本轮不执行这些动作。

两份实际retention延期、17版本前后真实native验收、剩余六份实际删除继续留在#307，分别准备和授权；不能声称完整D1已验收，也不把它们自动升级为本PR代码收口的前置。额外provider、性能、遥测、通用可靠性和未来能力按AGENTS §17处理为follow-up。

最终交付为：绑定本地新HEAD的源码与非删除证据、独立结论、更新后的PR描述，以及区分“本地代码阻断”和“未发布/未跑新HEAD CI/未获合并授权”的清单。旧证据仍是历史事实；本契约不回写其中的NOT_RUN或NOT_RECEIVED。
