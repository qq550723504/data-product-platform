# #307 D1 P1 缺失后的 17 版本完整只读合同

本阶段落实人类已要求的“先可信闭合旧 dispatch，再做17版本全量只读核验，准备剩余六份”。旧三项已由非删除audit `94f28318-cfd4-4c62-ba1a-bec9f35836c8` 实际闭合并独立接受；P1已由audit `d64ad338-00d2-4569-bd62-9e132ddaf578` 留下新absence `b2d74d75-ed07-4b0a-b530-6c5d308cf1b8` 和completion。上述旧事实不覆盖、不重复签发或重新调用；新读取使用独立audit、profile、owner和原始请求通道。当前不生成删除授权、不执行DELETE、不合并，#332 Draft、#307 OPEN。

现有 `remaining-verify-native` 把应存在版本与IAM policy预期资源共用18版本清单，不能表达已经合法删除的P1。新路径将三个集合分开：原冻结18版本用于解释旧policy；唯一已确认缺失的business P1用于新的exact HEAD及全namespace缺失核对；其余17版本用于全部实际bytes/SHA/version/hold/retention/完整库存核验。不能伪造P1对象、把17投影成18、跳过其他missing版本或更改旧validator含义。

复用已有官方MinIO SDK、GET/HEAD-only transport、policy ParseConfig/Equals、namespace versions分页、Core REPEATABLE READ READ ONLY/fingerprint、Docker CLI和固定helper分配模板。只在CLI/fixture组合层加入显式的post-v22只读模式与file-only聚合；不新增Core实体、队列、workflow或IAM引擎，不把SDK引入Core Domain。

新的技术只读profile只携带需要的源scope、Root技术凭据（provider侧仍具root权限，实际只读由固定GET/HEAD helper/transport强制）、inventory摘要、audit/时限和binary/CID绑定；不挂数据库DSN、业务Token、reader/producer secret、删除permission、旧人类授权、budget或宿主私钥。Root技术凭据仅位于已冻结GET/HEAD模式的维护helper，不能进入业务worker。实际命令、UID10001、cap-drop ALL、no-new-privileges、只读rootfs、两tmpfs、三bind、无端口/无socket/无restart和唯一绑定Native netns必须读回；禁止PG镜像自动卷。

输入保持原冻结candidate12/bindings及18版本receipt/policy原字节，用原manifest的Copy身份及两个已绑定Processing输出确定present17。P1必须是固定copy `5a4c9789-3e67-4e54-ab58-128d111cbd65` 的完整旧identity；fresh exact HEAD只能接受官方NoSuchVersion/NoSuchKey 404语义，并保存真实version selector、start/observation和错误code，不能拿普通404代替。完整version inventory也必须排除它；任何其他missing/extra/delete marker/内容或保护变化都失败。

全部17份都要实际GET并核bytes/SHA及provider version，再读取各自hold/retention。IAM仍核三源全部disabled identities、单policy/no groups和完整官方policy等价，原P1仍在旧disabled policy中的事实保留；这一步不修改或enable任何IAM。新的六份发送policy将在后续候选中单独收窄及冻结，不能把本次旧policy读取当成新发送授权。

宿主固定完整入口先核旧87唯一CID关闭、旧控制/只读profile、九备份、三旧预算及其完整挂载归属、三个sealed gateway和internal networks。business Core的基准是可信闭合03的实际Core-after SHA `74e45ae37d0dd4e998e461624dcddfffa80116a08cf3234a7e00dd0ad1fce6f8`；另外两源保持原冻结Core基准。逐组只启动绑定Core/Native和本组新技术helper，前后全Core fingerprint/roles/ACL相同，SQL只读，不迁移、不seed、不写业务事实。恢复/checkpoint CID必须与源分离并保持关闭。

profile在实际运行时只允许Enabled及NotBefore/ExpiresAt激活；run在全部fresh门禁前只生成一次统一NotBefore/ExpiresAt，三组共用同一对值，全audit期限最多55分钟，不能逐组平移截止、MaxProviderDeletes明确0；这是非删除读取期限，不是新的删除窗口或旧期限延长。新source/binary/输入/实际owner须先非删除回归和独立评审再运行；回执目录O_EXCL。旧预算不重置、不退还，新helper不挂预算。任何失败停止后续采集，保留原通道/部分结果，逐项关闭所有本轮helper及attempted sources；缺失为UNKNOWN/NOT_RUN，不能填布尔值或补造新鲜证据。无enable路径时不借只读核验修改IAM；无法确认disabled时标UNCONFIRMED并保持源物理停止。

file-only checker须同时验证：原18原字节基准、唯一P1缺失identity及实际HEAD日志、显式present17输入及所有实际receipt、完整policy成员及官方等价、当前audit/输入/binary/owner/时间绑定。policy排序变化继续接受，扩权继续拒绝。旧18 checker/模式保持原合同；新聚合单独输出明确17计数，不接受旧18回执、拼接旧fresh数据或漏一项的摘要。

有限验收包括：17正例和唯一P1 absence；错误missing/extra/version/bytes/SHA/hold/retention/marker拒绝；P1出现、错误copy/namespace/project/version/404语义/selector拒绝；policy重排通过、扩权/多policy/groups拒绝；来源/时限/未知字段/数据库或业务credential扩张拒绝；完整host成功与各类失败finally-close、预算/挂载归属漂移拒绝。mock、真实native、Core只读和独立评审分别留证。所有技术请求先start再observation，quantity按实际记录，金额未知NOT_ASSESSED，不伪称零外部调用。

本阶段PASS只证明17版本当前只读状态。六份新候选还需非删除逐份“send→关闭发送→absence→下一份”组合、source/binary/Core/IAM/gateway/新空预算/完整backup-restore绑定及独立评审；通过后才冻结新的execution和完整确认方案，再选未来固定窗口及取得新精确授权。P1永久不回目标，十一survivor、全部其他环境和备份、旧pins/预算/失败证据保留，完整D1仍NOT_ACCEPTED。
