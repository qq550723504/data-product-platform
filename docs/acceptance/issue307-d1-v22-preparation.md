# #307 D1 candidate12/v22 非删除准备与收敛矩阵

当前：**PREPARED_NOT_AUTHORIZED；完整D1 NOT_ACCEPTED**。固定完整入口已在真实保留环境成功跑通。新execution `8e41cfba-a41e-44dd-9ce8-d6365b3bb90e`；代理提议窗口为2026-10-19新加坡14:00–14:40（UTC06:00≤now<06:40），仅准备方案，尚非人类选择或授权。只读恢复上限UTC08:40。无新授权、无调度、发送权限关闭、真实DELETE=0。

完整范围见[精确确认方案](issue307-d1-candidate12-confirmation.md)、[有限手册](issue307-d1-v22-window-runbook.md)和[修复契约](issue307-d1-complete-preflight-contract.md)。#323保持Draft、不合并；#307保持OPEN。旧v20/v21授权、失败、预算、旧pins和所有环境/备份原样保留；不恢复旧execution或复用授权。

| 必需项 | 结果与边界 |
| --- | --- |
| 根因修复 | Docker原始wrapper统一bytes；仅docker_text严格UTF-8解码。case、私有文件白名单、operator/root实际隔离、internal network、预算归属/全部挂载者/空预算均进入固定完整CLI；源启动前拒绝异常。复用现有Docker/reader/官方MinIO checker/finally-close，无新增基础设施。 |
| 完整main成功与反例 | normal/-O各15方法通过：成功组合经过全部3组真实Python collector和Go file checker；str/无效UTF8、错budget owner、额外/缺失挂载者、非空预算、case挂载/网络扩张、私有目录泄漏，以及8个完整回执反例均拒绝。此项是recording/文件回放，不冒充原生运行。 |
| 既有回归 | freeze关闭故障 normal/-O各2；policy语义/排序/扩权各5；R1跨字段窗口各4（含15拒绝子案例）；Go5包PASS，旧07/09/10/11授权拒绝。 |
| 源码独立评审 | complete-entry review01源码报告PASS；B1私有目录凭据泄漏、B2回归集污染和G1完整回执空集合等有效阻断已闭合，原失败/初评保留。generation-binding addendum PASS。 |
| 冻结 | source `5c7e964dac83ebda0cf44743d49a546123735119`，831源码；packet anchor `30e954027bc1d5a74d683f0dcd8bd141aa7a678b`，只供封存定位。candidate SHA `f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243`；template SHA `bd46b276a422f75aa7e78b0568ab73b8412e265ad40897ce9fea2badc3645341`。 |
| 冻结后实际完整原生入口 | `remaining-candidate12/complete-entry/complete.json` 于2026-10-18T14:52:22Z PASS；实际subprocess exit0，entry SHA与冻结build一致。此前构建所需局部采集单列保留，不代替本项完整CLI。 |
| 宿主固定前置 | 39实际owner（17 operator/wire/recovery、15 root、7 case）、32权限false、7 case和7只读recovery、9备份、3internal network、3新execution预算且完整挂载CID集合精确/空目录；host_gate与独立原字节回执一致。 |
| 三源Core | 真实READ ONLY REPEATABLE READ前后指纹、roles/ACL均等于冻结baseline；恢复Core/native CID与源分离。六expected-head未变；本轮源Core Renew/业务未执行。 |
| 原生库存/保护/IAM | 18 exact版本bytes/SHA/full inventory/hold/retention与冻结一致，7目标/11保留。完整disabled身份、单policy/no groups、官方ParseConfig/Equals通过；policy原始JSON排序差异允许，语义扩权拒绝。 |
| 当前离线审计 | normal/-O均831源码/1305 raw PASS；有限3组/18版本/76读取/宿主gate及完整调用出口均验证，checker自身provider I/O=0。 |
| 真实技术读取/成本 | 两轮真实SDK采集152 unique GET，加固定root check-disabled16 admin GET，共168次真实技术请求；HTTP200=106，HTTP400=62（官方识别NoSuchObjectLockConfiguration，不当作absence）；PUT/DELETE=0，amount=NOT_ASSESSED。不能称零provider流量或零成本。 |
| 实际关闭只读读回 | 300保留CID全部stopped/PID0/noports/norestart；07–12合计192权限false。本代未materialize human文本、无authorized-execution01；旧授权文本保留。3gateway sealed，9备份精确；补充审计两次纯读取错误已保存及纠正，不是完整入口失败。 |
| v21失败证据 | 原窗口临时str.decode错误、stop/terminal-final/失败封存、251CID与三组post-seal IAM UNCONFIRMED原样保留（commit77e2a77b）。本次新只读disabled IAM证明不回写旧失败时点。 |
| 冻结后独立复评 | 见 `remaining-window-review06/independent-final-review.json` 及其证据索引；最终报告须PASS且remaining_blockers=[]后方可将方案交付供核对。 |
| GitHub | 发布后精确HEAD的PR CI与Draft/Open/线程读回另存publication-checkpoint；未发布前不借旧8eef绿色声称本轮CI。PR CI不是main CI。 |
| 仍未验收 | 源Core六Renew、P1/P2/T1/T2真实业务与发送、H0/H1闭合、H2实际204后UNKNOWN、M1混合检查点及恢复均NOT_RUN；完整D1 NOT_ACCEPTED。 |

本切片收敛边界：完整入口、类型/权限/预算/挂载/历史证据等必需路径与CI失败构成阻断；新产品能力、额外可选硬化不扩入本轮。下一步仅允许人类核对完整精确方案；必须另获新精确授权且固定窗口内新鲜前置全部通过，才允许进入有限执行。失败/过窗封存，UNKNOWN不重发；cleanup=0、hold-release=0、bypass=0。
