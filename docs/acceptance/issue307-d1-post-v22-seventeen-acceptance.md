# #307 D1 P1缺失后17版本只读验收

实际audit `f4330324-98de-4fc4-ae3d-e1feab7adaae`，运行HEAD `9708a54d44346906e8cff89bfbd77ec8e6ed28d1`。本轮仅读取，不启用发送、不执行DELETE或业务mutation。完整D1仍NOT_ACCEPTED，剩余六份尚未冻结/授权。

| 项目 | 实际结果 |
|---|---|
| 完整版本/bytes/SHA/hold/retention | business7+late6+mixed4=17，六目标及十一survivor全部匹配 |
| P1 | 单次exact version HEAD，真实provider NoSuchVersion/404及request ID；完整namespace亦缺失 |
| IAM | 八绑定身份disabled、单policy/no groups，八完整policy经官方ParseConfig/Equals通过；保留旧18资源policy历史 |
| Core | 三源before=after=各自权威基准，全部relation/roles/ACL一致；业务事实新增0 |
| 技术调用 | 74start+74observation，73GET+1HEAD；44×200、29×预期400、1×P1 404；金额NOT_ASSESSED |
| 保全 | 旧87+新3 CID全部停止/PID0；旧46control、八只读profile、九备份、三旧预算及挂载归属保持；新三profile恢复原false |
| source | 1914原始源码文件另存私有ZIP，逐成员SHA匹配冻结manifest；两binary的buildinfo无vcs.revision，不声称其单独证明Git SHA |
| CI | 精确运行HEAD的PR CI39262723502、event=pull_request、13/13SUCCESS；不是main CI |

源码独立终评SHA `b864aecf9dd0f9067c4a4448415e2d79b2fc37bb9d3059940206103fa6367784`；实际manifest复评SHA `edb1d915e9101485ad77c11f8064fe598ce2223833b44c20e0649713805472bb`；实际结果终评SHA `c6508108d16f6095889b7dce66bb26d4940073faf1934a8b9b1fed9ec69ab616`，三者均PASS且remaining_blockers为空。结果复评重跑的是file-only聚合与保全读取，不伪称再次独立GET原内容或另存了IAM user响应body。

原终态SHA `c6e12ffdb14b64c139d7352f67b6c54b1c3ff859fe0400f03c3343dcd4c92a33`；原execution index01 SHA `13601fe7a46de3dc2752f325a0e3d94362ea58a7d6d88e24158d6ca775fea236`，297原成员逐SHA通过。原始字节均不覆盖。group中的`provider_io=0`是file-only checker自身的字段，仅指聚合器不调用provider；实际本轮技术调用为74，不能用该字段声称整轮零调用/零成本。

独立评审首轮harness曾漏mock共享p1 wrapper，对合成名称one/two发起四次失败stop；原日志、失败私有诊断及修后全局argv拒绝边界报告保留。未操作批准CID或启动源，不把它隐瞒成首轮全程mock。源码半写快照的失败、原失败/旧预算/pins也全部保留。

新窗口仅按人类选择准备2026-10-20新加坡14:00–14:40（UTC06:00–06:40）。这是窗口偏好，不是新的删除授权。六份准备合同见`issue307-d1-post-v22-six-contract.md`；#332保持Draft、不合并，#307 OPEN。
