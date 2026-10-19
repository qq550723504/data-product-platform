# #307 D1 P1独立只读对账合同

本切片承接 `issue307-d1-post-v22-repair-acceptance.md`。人类在本聊天同意“完成P1只读对账，再依据新库存准备剩余六份方案”；这是新的只读处理范围，不能重开v22 execution或使用过期的删除/读取许可。R1/R2已独立收口；本文件单独约束P1，不增加DELETE、dispatch、Renew、业务重跑或hold/retention mutation。

唯一目标为business-races workspace `a4aa2689-6e8f-4180-a567-dfb875407eac`，原P1 copy `5a4c9789-3e67-4e54-ab58-128d111cbd65`，原attempt `009f4317-8855-4f99-80ec-cafc225f654e`，`dpp-lifecycle307-online/fixture/p1.csv@fed0102b-bc8b-4f47-b9e2-fae8d5c563ce`。原生/Core源仍为candidate12冻结的business Core/Native。原204/CONFIRMED ACCEPTED、预算和失败终态不改写；新增只读audit ID不得等于旧execution。

复用现有MinIO官方Admin API/ParseConfig/Equals、StorageInspector、RecoveryService、Transactional Outbox及Asynq。现有Outbox增加可选的服务端 `OnlyEventID`，在认领和租约更新前过滤；未指定时保持全局publisher。P1必须使用本次RequestRecovery的新event/work，不能靠handler拒绝来声称其他旧obligation未被修改，也不能直接跳过Outbox调用worker。

新的只读期限最多40分钟，根权限与worker配置分开，二进制SHA、实际固定CID/image/command/挂载/netns/noports/no-root/no-socket/no-restart与源/备份清单由宿主在启用前核对。只读bootstrap的新许可只允许临时启用 `inspect808a69606c36`；源全部四名用户先必须disabled，单policy、no groups及官方policy语义完整一致。delete、业务reader和producer始终disabled；旧case、sender、wire、root enable owners不启动，旧32permission仍false。root secret不挂worker，旧许可/候选/模板/二进制不改写。

worker仅有新的Reconcile/Inspect/Verify能力，配置不接受Root/Delete/seed/copy等字段。新独立Redis与所选work/Asynq task ID绑定。先确认原P1 observation并尚无absence，再用实际只读Port核七份保留版本的精确HEAD、原生保护及两个namespace完整LIST，确认P1 exact版本缺失；随后RequestRecovery→精确Outbox→真实Asynq→既有Core absence/finalization。P1前检只核HEAD/保护/完整库存，不声称重新读取了全部bytes/SHA；未来六份可执行候选仍必须重新做全量新鲜核验。

任何失败停止处理并保留原始错误/HTTP调用start与observation/真实成本数量（amount未知为NOT_ASSESSED）。关闭新worker/Redis和root enabler，固定disable-all并fresh读回四用户disabled，再停止原生与Core、逐项PID0核验；关闭失败继续其他关闭并标UNCONFIRMED。不会删容器、volume、对象、预算或备份，不释放旧pins、不重发原DELETE、不覆盖旧失败事实。完成只追加本次只读终态和矩阵；原v22终态与NOT_RUN快照保留。

本切片的接受条件为：配置/身份扩权反例、完整库存成功与反例、精确Outbox真实PG回归、独立复评通过，再运行绑定的新只读审计并确认P1 Core receipt、原observation/成本/旧预算仍存、所有资源封存。剩余六份仅在此后形成新范围稿；新删除候选/execution/固定窗口与全部冻结绑定须另行确认授权。#323 Draft、不合并；#307 OPEN；完整D1不提前验收。
