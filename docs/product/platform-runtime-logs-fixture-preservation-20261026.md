# 日志验收夹具容器保留检查修复

2026-10-26，修复起点为PR #363 `b37d03477bf0c8b6f54ce5ecd1b8fac10e8c22b8`。仅处理验收夹具的容器元数据比较、诊断与回归，不开发业务能力。真实来源仍未配置，默认预览未接入；原库、OIDC/RP、Agent API及预览保留，业务写入口关闭，不接模型、不派发真实执行，Draft不合并。

## 失败证据和归因边界

[旧CI作业121408207443](https://github.com/qq550723504/data-product-platform/actions/runs/40272503349/job/121408207443)的原管理51项验收已通过，随后在准备独立日志夹具时，`bootstrap.py`对原管理测试容器元数据的保留断言失败。原`Mounts`数组直接参与Python字典相等比较，没有安全的before/after差异输出；日志18项与同宿主设置51项被跳过，旧FAIL及NOT_RUN记录保留。

旧作业记录runner镜像`ubuntu24/20261012.334`，其[固定软件清单](https://github.com/actions/runner-images/blob/ubuntu24/20261012.334/images/ubuntu/Ubuntu2404-Readme.md)列出Docker Server 28.0.4。本机Server为29.8.2。Moby的[上游修复ff0b471f20](https://github.com/moby/moby/commit/ff0b471f20)说明挂载来自map，旧inspect可任意排列，之后才加入排序。该已知机制与本机未复现、旧runner版本不同相符；确定性回归重现了“挂载内容完全相同、仅顺序不同，旧比较仍失败”。

**旧作业究竟哪个字段不同仍是UNKNOWN。** 原作业没有保存差异，不能据此追认原容器绝对未变，或把旧FAIL改为PASS。新CI会对自己拥有的管理夹具做65次只读观察，保留顺序变化与规范比较结果，作为独立机制见证；最新结果以对应HEAD的报告和收口评论为准。

## 修复与复用

复用仓库[已有保留验证的挂载排序做法](../../tests/fork-host-live/verify-preserved.mjs)，Python实现集中于[fixture_preservation.py](../../tests/fixture_preservation.py)，使用标准库JSON、SHA256和Counter；没有新依赖、Docker封装服务或应用基础设施。1A准备脚本同样使用原始挂载数组与`assert`，因此将它与管理/日志准备脚本接到同一检查，保留1A原有image字段，避免同根问题留在上一阶段。

- 仅把挂载列表按完整规范JSON排序；所有已有/新增/嵌套挂载字段、值类型和重复项都参与比较。没有按Destination建map或去重，不解析/折叠路径、别名或大小写。
- ID、status、startedAt、1A的image及挂载增删、Source/Destination/Type/Mode/RW/Propagation/Driver/Name和扩展字段变化仍拒绝。使用显式异常，不依赖可被Python `-O`移除的断言。
- 只有成功的精确名称容器列表能证明不存在；再按完整容器IDinspect并核对Name/ID。daemon错误、权限拒绝、超时、模糊结果、容器在两次观察间消失或非法JSON均不能转换成`None`后通过。观察不可用记UNKNOWN并阻断，没有隐式重试。
- before原始快照继续仅保存在既有私有验收目录；公开诊断固定为逻辑资源名、present、指纹、挂载数量、changedFields、挂载增删的指纹/重复数和mountOrderOnly。原始inspect、环境、配置、host路径、卷名、credential和daemon原始错误不输出/上传，未知异常仅记录固定错误码。
- 真实变化输出FAIL，观察/诊断不可用同样失败；不重建容器、不延长凭据、不重置会话或源时间、不清库解决检查。准备脚本仍拒绝已有目录、容器或占用端口。

## 验证

| 验证 | 本地结果 |
|---|---|
| 标准库单元回归 | 13项PASS；含旧比较的顺序反例、16类真实字段变化、挂载重复数量、image/出现/消失、类型差异、快照/观察失败及诊断写失败 |
| 优化Python | 相同13项在`python -O`下PASS，真实变化仍显式阻断 |
| 诊断泄漏反例 | 嵌套字段、源路径、卷名、Config/Env及异常中的PRIVATE哨兵不出现在安全报告中 |
| 只读实际Docker观察 | 本机管理验收容器65次，rawVariants=1、canonicalVariants=1、无变化；没有原库SQL、容器启停或预览改动 |
| 语法/实现范围 | Python解析通过，保留路径无可优化掉的assert；不修改apps、迁移、依赖、业务接口或来源/授权设计 |

CI增加普通/优化Python回归，在原管理夹具完成后运行只读观察；1A、管理与日志准备分别保存`retained-comparison.json`。artifact只追加这些固定安全文件及`retained-order-probe.json`，不上传原始快照、profile、配置、证书、密钥或完整`.data`。检查失败仍阻断后续日志验收；不能把警告、UNKNOWN或跳过记作成功。

[本地安全观察](platform-runtime-logs-fixture-preservation-20261026/local-readonly-probe.json) · [回归摘要](platform-runtime-logs-fixture-preservation-20261026/results.json) · [字节校验](platform-runtime-logs-fixture-preservation-20261026/manifest.json)。本地观察没有出现重排，不冒充旧runner复现；新CI是否观察到顺序变化会明确记录，不以测试数量推定。

重演单元：`python -B -m unittest discover -s tests -p test_fixture_preservation.py -v`，再用`python -O -B`运行同一路径，不需要Docker、数据库或凭据。实际只读观察脚本为`tests/platform-management/probe-preservation.py`，仅接受既有独立夹具ack和固定管理容器，新结果目录存在即拒绝覆盖。不要为重演删除旧结果或重置任何资源；新完整夹具在全新独立CI中按原流程准备。
