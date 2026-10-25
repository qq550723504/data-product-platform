# #307 D1 旧 dispatch 闭合验收矩阵

本矩阵记录新的非删除准备，不重开 v22；#323 Draft，#307 OPEN，真实 DELETE=0。本切片仍未完成，不能生成可执行候选或精确授权文本。

| 项目 | 实际结果 | 证据/限制 |
| --- | --- | --- |
| Docker 恢复后保全 | PASS | 77 CID 停止/PID0/noports/no-restart；46 旧控制文件、8 只读 profile、9 备份、3 预算组与原 anchor 一致 |
| 固定只读审计入口回归 | PASS | normal/-O 各 11 项；Docker/Core 为 mock，另有独立预算缺失及 SHA 漂移完整入口反例 |
| 审计入口独立复评 | PASS，blockers=[] | source-final-review01.json SHA a84cb16d31f96d2c62d0ef6bea79d2a9d4e5439d2135d1ba8be679c9deeff5fb；只覆盖审计入口/合同，不覆盖后续 mutation adapter |
| 实际 Core 只读审计 | PASS | audit 23587686-0a95-4eaa-a010-a8c896c9735b；REPEATABLE READ READ ONLY；全 relation/roles/ACL 前后一致；Core 最终 PID0，全部保全内容不变 |
| P2/T1/T2 旧占用 | 已逐项查实，尚未闭合 | 各 1 dispatch/attempt、0 claim/observation/release/absence、unresolved=1；三项旧 approval 均未撤销；T1 的 3 行 reader pins 原样保留 |
| 新鲜可信 owner/IAM/gateway proof | NOT_RUN | 当前审计不启动 Native/IAM helper、不签名；无冲突不等于可信 NOT_SENT |
| 固定 NOT_SENT adapter | DRAFT_NOT_REVIEWED | post_v22_no_send.go 初稿，未接执行入口、未做专属回归/PG验收，禁止据此运行 |
| 原源 revoke→NOT_SENT→release | NOT_RUN | 必须先完成新有限 authority、宿主采集、专属反例与 PG 回归、独立评审；仅复用现有 Command，不直接 SQL 插事实 |
| 17 版本全量只读 | NOT_RUN | 待三项可信闭合后；P1 不再进入目标，六目标+十一 survivor 必须全量 bytes/SHA/保护/库存刷新 |
| 六份新候选/execution/窗口/确认文本 | NOT_FROZEN | 原 v22 授权不复用；没有新的可执行删除许可 |

实际审计原始行和 before/after/终态均保存在 `evidence/issue307-d1/post-v22-dispatch-audit/23587686-0a95-4eaa-a010-a8c896c9735b/`；旧 v22、P1 audit01–04、预算及 pins 均不覆盖。新审计无 provider 调用、无成本伪录、无业务事实写入。

当前代码与证据先隔离保存，供 #323 同步最新 main 的独立复评使用。不得把本切片尚未完成的工作混入已有 R1/R2/P1 的评审结论或冒充完整 D1 验收。

## 2026-10-19 宿主入口准备追加

前文保留为初次 audit 时点快照。#323 后续已依据协调聊天中的直接人类授权合并；实际 SHA `f22587d780bacc9c865b269fa20b1cb135034f5a`，push/main CI `39250314443` 13/13 SUCCESS，#307 仍 OPEN。下列新代码隔离在 `codex/issue307-post-v22-dispatch-closure`，未混入 #323。

| 项目 | 最新实际结果 | 证据/限制 |
| --- | --- | --- |
| 固定 NOT_SENT adapter/observer | 独立源码 PASS，blockers=[] | observer-source-review01.json；专属签名/时效/身份/DSN/capability 回归；真实PG eligibility反例另存 |
| 宿主完整入口回归 | normal/-O 各15项 PASS | host-composition01.json；包含成功、前置拒绝、部分提交、IAM失败、Native inspect失败、disable失败、逐项stop错误、原通道保留、挂载别名/子目录、gateway/budget漂移；全部为 mock |
| 实际 PostgreSQL 三组闭合事务组合 | PASS，0 skip | postgres-regression04.json；同 key replay；新增3/3/3和6/9/9/9/9；原pin/attempt/approval/dispatch/claim/Cost hash不变；synthetic历史reader termination在测试baseline前完成，原源未动 |
| PG harness/fixture 原失败 | 保留 | 02误读动态端口；03合成reader未先终止而被正确阻断；原回执/通道不覆盖，04分别纠正真实原因 |
| 39旧 helper 配置/CID/binary | 实际只读匹配、全部停止 | 原case必须保持关闭；仅内存反向既有 Enabled 撤销以匹配原SHA；未恢复旧profile，source start=0 |
| 新宿主源码终复评 | PENDING | host-draft-review01 的 HOST1/HOST2 已修，等待最终字节复评 |
| 新 stopped owner/manifest | NOT_PREPARED | 源码终复评并提交后才 prepare，manifest 另需独立评审 |
| 原源 revoke→NOT_SENT→release | NOT_RUN | 无签名、无原 Core 业务写入；旧预算和 pins 保留 |
| 17版本全量只读及六份新候选 | NOT_RUN / NOT_FROZEN | 不复用旧授权；没有新删除 execution、窗口或可执行许可 |

固定宿主合同见 `issue307-d1-post-v22-dispatch-closure-host.md`。源码/manifest/runtime 分层验收，完整 D1 仍 NOT_ACCEPTED。

最后一次源码补齐：启动前 source topology 已收进固定入口，normal/-O 各17项 PASS，见 `host-composition02.json`。旧 `host-source-final-review01.json` 只绑定前一版字节，其 superseded 回执明确不可用于当前 prepare/run；最终02复评仍待完成，原三项 dispatch 仍 NOT_RUN。

最终源码复评已完成：`observer-independent/host-source-final-review02.json` SHA256 `a0a2c6291a58ce499af7d81630b0c5276d48b35e8afbaaf34a5b023878181730`，status=PASS_BOUNDED_INDEPENDENT_OLD_DISPATCH_CLOSURE_HOST_SOURCE_REVIEW、remaining_blockers=[]。HOST1/2/3闭合；独立 normal/-O 各17项 PASS。接下来仅准备 stopped owners，再评实际 manifest；source PASS 不代替 manifest/run gate，原源三项闭合仍 NOT_RUN。

第一次实际 prepare 保留于 `post-v22-business-closure01`：observer 的实际 Docker Config.Cmd=null，清单期待 []，最终配置检查正确拒绝。原源启动/签proof/业务写入/DELETE均0；三份新CID均stopped/PID0，profilefalse，proof不存在。最初终态诊断误称“继承postgres”，`diagnosis-correction01.json` 明确更正为 null/空数组表示差异，原字节不覆盖。当前入口改为显式固定非空 observer 参数，空参数在任何Docker前拒绝；下一次非删除审计使用独立 closure02，并在所有门禁核对失败01的清单/终态/输入哈希及三份CID关闭状态。normal/-O各20项PASS见 `host-composition04.json`；最新源码与旧02不同，等待有限03复评，不能用旧报告运行。

本轮已触发 AGENTS §17 收敛检查点：有限合同冻结为旧三项 dispatch 的可信 NOT_SENT 闭合、原事实/预算/pins 保全、明确 partial/UNKNOWN、完整 finally 封存。仅当前真实 command 契约缺陷及失败资源保全属于此次复评 BLOCKER；未来扩展和通用 hardening 不扩大本切片。17版本刷新及六份候选仍后续门禁，原源三项仍 NOT_RUN。

prepare02仍被同一完整 command/mount 检查拒绝，原源启动/签名/业务写入仍0：argv已正确，但PG镜像 VOLUME自动创建第四个挂载。根因是新allocator没有完整复用已验收P1的两个tmpfs，现已整体采用原模板，并精确核对Tmpfs及仅3bind；没有允许额外卷绕过检查。02清单/终态、六旧新CID及六匿名卷均保留，01诊断追加更正02说明组合条件的两个失败分量。实际 Docker stopped allocation 回归 `docker-allocation-contract01.json` 已PASS：固定argv、两tmpfs、三bind、0匿名卷、从未启动，helper CID `39cf3079786230c50f25c51062fdb12fbec9860f6957bd690466cc03ce3cd578`。下次03在门禁中同时保全两失败和此回归owner。normal/-O各20项仍PASS（新增tmpfs缺失/volume扩张子案例），`host-composition05.json`；等待有限04源码复评，03实际准备及原Core闭合均NOT_RUN。

## 实际闭合与独立终评

非删除 audit `94f28318-cfd4-4c62-ba1a-bec9f35836c8` 已于 2026-10-19T13:00Z 实际完成。冻结运行 HEAD 为 `375ae03cee8789708d08dca84a34bf102cc094d6`；prepare03 manifest SHA `b6b92f1e87714799293f1cf68486e48802650741f985f11278aa60ed867c2684`。source04、manifest03、result03 是本轮实际依据，前文旧状态保留为各时点事实，不再代表当前状态。

| 项目 | 最新实际结果 | 精确证据/限制 |
| --- | --- | --- |
| 源码04独立复评 | PASS，blockers=[] | report SHA `d32e20637b49151276fa73bfdae1b46a64cffc061c40998490b68a2be4daf16f`；normal/-O各20项；实际Docker分配模板及所有失败保全已核 |
| 实际manifest03独立评审 | PASS，blockers=[] | report SHA `40783374024610fa8179ce125514a57b0257b340003a7b3e5feea220df924a62`；605源码/10输入/3新owner/所有旧资源精确匹配；Go metadata没有vcs.revision，HEAD、源码、build recipe和binary SHA分别绑定，不虚称embedded revision |
| 原源三项可信闭合 | PASS_TRUSTED_OLD_DISPATCH_NOT_SENT_SEALED | terminal SHA `3ce05b24b298718b63dcd2c553c993b6f40a387c3a0eb26e13fda695ed5692cc`；各1新approval revocation/NOT_SENT/release，3项unresolved=0；没有claim或absence |
| Core完整增量 | PASS | 仅9预声明关系变化；revocation/observation/release各3、idempotency6、Audit/Evidence/Relation/Outbox各9；Core-after SHA `74e45ae37d0dd4e998e461624dcddfffa80116a08cf3234a7e00dd0ad1fce6f8`；roles/ACL及其他关系不变 |
| 签名、IAM与物理封存 | PASS | trusted evidence SHA `6ba325a75b775e6977c471852439a028928a3a0c1db5bcdfa7f519322275632b`；8真实GET技术请求/原观察；无enable；87个唯一CID停止/PID0，当前profilefalse，旧46controls/8profiles/9backups/3budgets/6匿名卷保持 |
| 原行/旧pin/预算/成本 | PASS，有证据边界 | 全部旧append multiset由冻结host的实际内存gate验证；before anchors未归档，不能补造或声称独立从raw重算全部。独立另重算全部relation delta、旧三组attempt/dispatch/approval及T1三pins逐字节保持、最终anchors |
| 实际结果独立终评 | PASS，blockers=[] | `result-final-review03.json` SHA `33bcf7eb3fd742d932996f5378cfa9870cca2e9009245f5b815152d485b7c80b`；index01的31成员和追加index02的40成员/8份原owner通道均闭合 |
| 调用/费用/删除 | 分开记录 | 业务provider invocation=0，DELETE=0，pins released=0，budget refund=0；8次真实技术IAM请求，amount=NOT_ASSESSED，不能说所有外部调用/成本为0 |
| 17版本全量只读 | NOT_RUN | 六目标+十一survivor及P1独立absence、三源Core/IAM/备份/封存尚待完整新入口，不能投影旧18版本PASS |
| 新六份候选/窗口/授权 | NOT_FROZEN / NOT_RECEIVED | 不复用v22授权；完整D1仍NOT_ACCEPTED |

P2新NOT_SENT `92521ce8-2486-4b91-80ac-a14202de8786`，T1 `ac82beb2-cccd-4c82-ab90-2a941a0f7294`，T2 `7f7dc79d-28fc-4ca1-944a-8e4a5e76860c`。所有运行原字节及追加索引见 `post-v22-business-closure03/`，两次失败01/02不覆盖、不删除。后续代码在 Draft #332，#307 OPEN，不合并、不执行真实删除。
