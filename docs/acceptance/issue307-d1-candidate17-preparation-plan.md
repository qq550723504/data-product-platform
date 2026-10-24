# #307 D1 candidate17/v27 非删除准备方案

状态：PREPARATION_PLAN_ONLY，尚未分配或冻结新候选，不能用于删除授权。新execution `6f40bca9-cfe2-4362-ad2b-4e38bbbd8453`。人类已选择2026-10-21新加坡14:00–14:40（UTC06:00≤now<06:40），只读恢复上限08:40Z。窗口选择不是执行授权；authorization=NOT_RECEIVED，schedule=NOT_CREATED。#332 Draft、不合并；#307 OPEN。

## 前置修复已独立闭合

源码修复 `c1dd1b61495fd3150052744d6ba91e096a428b99`，独立报告 `b8e09798480023618c5af0451b67ea8c17e859d52e4fe26449f8ec661cfc5183`，remaining_blockers=[]。B1真实父junction终态反例与B2 Windows 8.3路径成功组合均已复验。normal/-O各52方法，51PASS/1明确symlink账户权限SKIP；实际历史84成员核验不是新native验收。

## 新候选必须先解决的代际接线

现行生成器及Go编译常量仍严格限定v26，不能只改permission或时间就重跑。该拒绝门禁保持有效。新准备将明确新增本execution/统一START-END绑定、保留已终结v26及其六case/六recovery撤销事实的审计，以及完整旧IAM身份集合；不能把旧v26当作未运行、没有授权的准备快照。它不改变业务语义、Core迁移、六目标顺序或11保留版本。

1. 原v26失败/103步seal/445CID物理读回和新独立复评全部保留，旧预算、pins、备份、用户原文/envelope及全部provider事实不改写。旧v26整体IAM关闭仍是UNCONFIRMED；下次完整native检查须从实际服务重新取得证明，不能复用旧布尔值。
2. 先完成新代编译绑定及retained-v26的有限门禁接线和离线回归，再分配全新停止owner、恢复卷/网络和空预算。原源数据版本不重建，不恢复旧execution、IAM secret、预算或pins。
3. 三源Core只读指纹/roles/ACL、17份version bytes/SHA/full inventory/hold/retention、完整旧/新disabled IAM、官方policy Equals、独立backup/restore与业务rehearsal分别取新回执。任何未运行项写NOT_RUN，不用当前历史清单冒充fresh。
4. 固定完整入口必须显式核验files-map-v1与members-list-v1。准备阶段只能核当时已经存在的索引；未来实际授权/调度取得后另生成并冻结对应完整audit plan，plan/index/成员SHA均进入精确确认及宿主批准引用，不能填空索引或临时审计脚本。
5. 新source、Linux binaries、Windows checker、Core/IAM/gateway、CID/mount/network/budget/backup及audit plan完成冻结和独立packet复评后，才生成完整精确确认文本。本文件不伪造这些尚不存在的binding，也不申请删除授权。

## 预期范围（旧冻结证据，仅供新核验逐项比对）

新准备的预期顺序仍为P2→T1→T2→H2A→M1A→M1B；每份最多一次、全局最多六次，UNKNOWN不重发。以下18历史copy中排除已确认缺失的P1；列的是旧冻结证据中的17现存版本，尚非新鲜结果。

| 范围 | project | namespace/key@version | copy |
| --- | --- | --- | --- |
| 目标 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302 | 6fabf3b5-76b8-4245-8f5d-01e64f610146 |
| 目标 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750 | f48755f1-976b-4f47-98da-70ead936a4c3 |
| 目标 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314 | e09fd433-54cf-4e82-b8c4-098472505fa2 |
| 目标 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc | c05307db-cb81-4220-b42e-be23f85821e3 |
| 目标 | dpp-lifecycle307-dfda43b4 | dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d | 5e712426-2109-4f0b-8791-df0a2be3a9f7 |
| 目标 | dpp-lifecycle307-dfda43b4 | dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6 | ad08947a-4c4d-4b0f-8c7e-2d89f287819f |
| 保留 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-online/fixture/control-hold.csv@47026891-d10a-4400-a3cb-76915e3d51bf | 91d954ee-cce5-4ceb-840d-fd6bdab778be |
| 保留 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-quarantine/fixture/control-retention.csv@2606d71f-207b-42a7-b671-4b7f4ca4d194 | aa8ecd43-1c17-4d0b-b462-42600a426b1b |
| 保留 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-online/fixture/remaining-processed.csv@00d7e220-7714-4c31-a4c7-2dda08cfa306 | 639c4cee-6a7c-4a13-aa68-37035d6e1811 |
| 保留 | dpp-lifecycle307-808a6960 | dpp-lifecycle307-online/fixture/remaining-standardized.csv@dac1dbc5-c9b5-412a-a2a6-bf082e109e22 | da6cc6a8-c9c2-41a6-8811-2d46164a241d |
| 保留 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-online/fixture/control-hold.csv@ca3b808e-734b-4d5e-8f2d-c78a16056ed6 | a01751a1-9311-4357-8fb2-c833de6b8fc4 |
| 保留 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-online/fixture/h0.csv@369808db-a39a-4327-b9ed-5eb83aef7477 | b3c4e113-f5c5-4421-a9d6-08403b1b9d0a |
| 保留 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-online/fixture/h1-a.csv@28d9f84a-1889-498d-b033-df26cbf209f2 | 0d8b77a8-195d-4a83-bd52-04f800d7dc68 |
| 保留 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-quarantine/fixture/h1-b.csv@bae40a85-7539-4fe1-86b7-2cbb17df7812 | e735d90a-8352-4a01-ad16-8573297bc684 |
| 保留 | dpp-lifecycle307-754ea8f0 | dpp-lifecycle307-quarantine/fixture/h2-b.csv@448112b2-baf3-4569-b06f-59011492cf80 | a37c347d-dbff-492e-9c7a-317a1f188184 |
| 保留 | dpp-lifecycle307-dfda43b4 | dpp-lifecycle307-online/fixture/control-hold.csv@a59df17d-70d7-40c5-88e8-8cfe77a915dd | f3167994-3790-4a6c-918e-ea6d95248b79 |
| 保留 | dpp-lifecycle307-dfda43b4 | dpp-lifecycle307-quarantine/fixture/m1-c.csv@81a8402e-390f-4247-9d34-552df5eadeb3 | 13fd8ecf-07fd-4981-b553-5ce66b67444f |

## 当前验收与冻结边界

| 项目 | 当前事实 |
| --- | --- |
| 索引修复及独立复评 | PASS，0剩余有效阻断 |
| 新execution/窗口偏好 | 已记录；非执行授权 |
| 新代配置/retained-v26接线 | 待实施并独立核对，不绕过现行只支持v26的guard |
| 新owner/卷/预算/IAM分配 | NOT_RUN |
| 三源17版本fresh及备份恢复 | NOT_RUN |
| 新候选source/binary/Core/IAM/gateway冻结 | NOT_RUN |
| 新完整精确确认文本 | NOT_READY，冻结及新packet复评后提供 |
| 人类删除授权/定时续跑 | NOT_RECEIVED / NOT_CREATED |
| 本轮真实DELETE | 0 |
| 完整D1 | NOT_ACCEPTED |

全部survivor、其他环境、备份和失败证据保留；cleanup=0、hold-release=0、bypass=0。实际未来执行仍须新的精确授权，固定窗口之外不得发送；失败或过窗封存，不延长、平移、重开。
