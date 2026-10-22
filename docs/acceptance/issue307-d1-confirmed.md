# #307/D1：获批 exact 删除、普通 reader 竞争与只读恢复

2026-10-13，接续[非删除检查点](issue307-d1.md)，基线 main79d175d0/schema66。
用户明确确认 `call_0275d4f95cd44342bf0c36afa69053e3` 的两个版本；此前
`PENDING_USER_CONFIRMATION / DELETE=0` 回执保留其原阶段，不改历史。

**PASS_SCOPED_EXACT_ABSENCE_AND_ORDINARY_READER_RACES。完整D1仍NOT_RUN，
真实Execution/认证DirectData处置竞争尚未闭合；人工解释继续暂缓。**

## 确切范围及独立备份

仅 project `dpp-lifecycle307-9812f0fc`、run `9812f0fc-c05a-4bcb-87ea-a4cef9089f3f`。
Plan `756a4139-bfae-4b5e-9368-e65d28d4c877`，hash
`fa2b19e1d7fc12132b3f4e4b4acee09e4f5bc0f59eae87820b83fb2fff220c86`。

| namespace / key | exact version / bytes | 结果 |
| --- | --- | --- |
| dpp-lifecycle307-online / fixture/a.csv | 0f6aa755-4b86-496b-9ee1-43d7c59a8bb3 / 232 | 1 DELETE，UNKNOWN原观察保留，exact absence receipt |
| dpp-lifecycle307-quarantine / fixture/b.csv | e1611b0a-efc0-4939-abc2-30394ec2c8d2 / 129328 | 1 DELETE，UNKNOWN原观察保留，exact absence receipt |

删除前新检查点独立恢复：192 relations/266 rows，完整fingerprint
`b01197897206cb3036fd087685018992e1819cc44436a963182673374e0148fb`，
owner/ACL/SCRAM正负例及四份 exact object version/size/SHA256 相符。
检查点包含候选plan；不声称它包含后来的批准/处置事实。备份/恢复副本保留，
含被删source的可恢复payload，明确排除在source receipt之外，不声称全域销毁。

## 原生实跑与账务

- 普通fixture reader先进入：真实MinIO GET和SHA256验证 → output barrier
  持有durable pin → 追加处置批准/使用限制 → dispatch被reader阻断、0 DELETE。
  writer明确结束、verified termination后才允许dispatch。
- 反向顺序：reservation提交后新reader 0 native GET/0 bytes。此为普通fixture
  reader竞争，不能冒充真实Execution或认证DirectData的LC-11/12。
- 每份target先DB dispatch/start + ClaimExactSend，原生IAM和网关均只允许获批
  key+version；无root挂载、裸key、bypass、COPY或hold/retention解除。
- 复用现有Caddy：对上游成功204用
  [handle_response](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)
  + abort制造真实下游响应丢失；SDK/transport单发送，Core记录CONFIRMED
  invocation/UNKNOWN outcome及成本。每次删除后真实进程exit93，未先补absence。
- 既有Outbox→Asynq→只读Worker恢复原attempt，分别取得exact inventory/absence；
  第一份只有1 receipt/0 completion，第二份才2 receipts/1 completion。
  原UNKNOWN观察不覆盖。终态重放及两个并发worker终态重放均新增native I/O=0，
  不宣称已跑in-flight worker owner/crash竞态。
- 独立SQL：44 storage calls/44 observations/44 typed costs + 2 DELETE
  attempts/send claims/UNKNOWN observations/typed costs，共46项data-plane invocation。
  两个pins/两个verified stops、active=0。IAM基础设施准备/禁用的私有START/OBSERVATION
  journal单列，不混进这46项业务数据面成本。
- 新Core的162/192 relation fingerprints不变；仅Lifecycle、fence、幂等、
  Cost/Evidence/Audit/Outbox追加或受控projection变化。DatasetVersion、Resource、
  Rights、Quality、Certification、Execution、Release全部原业务fingerprints不变。
- 最终真实inventory只剩两个受保护版本；identity、size、retention/hold与原manifest
  一致。native reader/delete用户已禁用，recovery authority disabled，网关仅health
  允许，其他对象/admin I/O deny-all；所有旧环境、卷和备份保留，cleanup=0。

## 必须保留的失败、源码与缺口

默认Docker地址池耗尽、备份脚本resume失败、admin journal只读挂载失败、
CopyService缺少required composition配置、批准已阻断新reader以及网关封存的指令
顺序错误均保留失败记录。前期未发出删除的helper失败时attempt/pin=0，没有实际删除；
只对确定未发出I/O的阶段作显式修正/继续，不盲重发unknown DELETE。

实际两次DELETE使用code `ced51ff187c28be6ff043307c228fc1cfd55513f`，binary
`0b10fa5b4135b4a1ff9a785c55dca3280e9e86bdd292a9a701707eecb301451d`。
后续`0c439930`二进制只执行read-only survivor检查和IAM禁用；driver封存修复另有
阶段hash。source checkout与Git blob hash分开记录，不声称最后文档HEAD重新删过对象。

原封存A环境只读SQL复核发现8类真实FK关系（Execution inputs、lineage、Quality、
Certification、Delivery等），pin停止后`InspectFixtureReferences`仍DENIED。
它保守地把每条业务FK作为占用，缺少reference owner批准的material-pin转移/释放事实，
因此当前代码无法据此完成真实业务的处置闭环。**未排除这些FK、未放宽gate，也未把
普通reader结果计入完整LC-11/12。** 下一项必须由相应reference owner明确仍需字节的
要求及受控处置事实，再验证真实业务双向竞争；当前native archive/restore rerun、
in-flight worker fault组合和完整D1仍未完成，不进入D2，不关闭#307。

证据：[summary](../../examples/lifecycle-d1/results/9812f0fc-c05a-4bcb-87ea-a4cef9089f3f/confirmed-race-summary.json)、
[source scope survivors](../../examples/lifecycle-d1/results/9812f0fc-c05a-4bcb-87ea-a4cef9089f3f/confirmed-race-survivors.json)、
[reference gap](../../examples/lifecycle-d1/results/9812f0fc-c05a-4bcb-87ea-a4cef9089f3f/confirmed-business-reference-gap.json)、
[frozen manifest](evidence/issue307-d1/receipt-manifest-confirmed.json)。PR保持Draft，不自动合并。
