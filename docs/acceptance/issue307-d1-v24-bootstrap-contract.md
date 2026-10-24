# #307 D1 disabled IAM 原生接线修复的有限设计

candidate13/v23 的非删除准备失败，不能冻结或授权。business 的新 inspector 已以 disabled 创建，GetUserInfo成功；随后 AttachPolicy返回NoSuchUser。原入口终态、六次真实技术请求、原日志、92项关闭证据和全部预算保留；完整17版本、新备份恢复和业务重跑均未完成。失败束为`remaining-candidate13-preparation-failure01/failure.json`，SHA`0ece4d5e2e5ced1acd0f60b3ca6f6ff8359bbf2c113c0bb10b1d364edc2537f9`。

## 根因和复用决策

固定MinIO binary SHA`fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`的实际buildinfo对应`github.com/minio/minio v0.0.0-20251015172955-9e49d5e7a648`。该提交的`PolicyDBUpdateBuiltin`和旧`SetPolicyForUserOrGroup`都经GetUser→CheckKey→Credentials.IsValid判断存在性，disabled用户会被排除；SetUserReq中的Policy也未在该版本创建路径采用。原模拟仅验证SDK请求，并未复现这条服务端语义，是实际验收准备的覆盖缺口。

复用当前madmin-go官方ImportIAMV2和标准archive/zip，不fork MinIO、不添加IAM系统，也不临时启用Source身份。该版本的user_mappings导入路径直接调用PolicyDBSet，允许对现存disabled用户关联policy。相同镜像的隔离API probe已真实复现两个旧API拒绝及单mapping导入后disabled/sole-policy/no-groups成功；该probe只证明API可行，生产受限transport和完整非删除验收仍须另行通过。

固定官方来源：[IAM判断](https://github.com/minio/minio/blob/9e49d5e7a648/cmd/iam.go)、[导入实现](https://github.com/minio/minio/blob/9e49d5e7a648/cmd/admin-handlers-users.go)。这属于既有SDK Adapter的有限接线修复，Core/Port/Outbox/Asynq/数据库语义不变，无新增基础设施。

## 当前新提案

candidate14/v24，execution `afc0e66c-535b-426e-a937-964d0c9ca0d6`。沿用人类选择的2026-10-20新加坡14:00–14:40（UTC06:00≤now<06:40），只读恢复最晚UTC08:40。该选择只是窗口偏好；本代删除授权NOT_RECEIVED、调度NOT_CREATED，不复用任何旧授权。六目标和十一survivor保持原精确版本，旧P1不进入目标或预算。

新提案使用全新源码/四binary、86个新CID、15新卷、独立nonce和恢复网络；旧candidate13的86CID、三空预算、private secret、partial身份和所有失败证据原样保留并关闭。不得绕过原checkpoint02的same-binary门禁：原链只解释c13历史，新c14直接从新提交构建并核完整12成员，使用单一严格当前contract，不加通用旧source fallback。

## 受限导入合同

每次仅允许固定localhost endpoint的PUT /minio/admin/v3/import-iam-v2，query为空。ZIP必须恰好一个regular成员`iam-assets/user_mappings.json`，使用标准ZIP存储，压缩和展开尺寸均有小上限；其JSON只能是一个精确本execution用户，值仅version=1及一个精确policy。拒绝重复成员、重复JSON key、额外成员/字段、路径、其他用户/policy、users/groups/STS/service-account材料。完整ZIP及单mapping SHA留证，不含secret/status。

新六用户各先确认不存在；policy用官方ParseConfig/Equals核其exact范围，官方SetUser(AccountDisabled)只调用一次，fresh GetUserInfo须disabled、无groups且无直接policy，再导入该用户单mapping。mutation key包含已验证的user/policy，每用户最多一次；SDK重放或UNKNOWN均拒绝，不重发。不得走DecryptData用户创建分支解释ZIP。

V2原始响应必须留存，并严格核top-level skipped/removed/added/failed字段、拒绝未知字段和尾随内容。所有Skipped/Removed/Failed分类为空；Added.UserPolicies恰好一个精确user→单policy，其余Added分类全空。随后fresh GetUserInfo必须disabled/no-groups/sole-policy，再核实际policy官方Equals；HTTP200/SDK error=nil不构成成功。

## 保留的真实partial身份

已实际创建的`i4e524c04808a6960`只属于失败c13。它不被新worker使用、不复制secret、不SetUser、不导入mapping、不enable。Source、Restore、完整预检和未来root都显式核它始终disabled、PolicyName严格空、groups严格空；仅此精确已证明的身份允许NONE模式，其他身份仍必须sole-policy。其已存在但未关联的`lifecycle307-six-i4e524c04808a6960`policy仍按固定bytes/官方Equals留证，明确为unassigned，不伪称已经关联。

本代实际known身份总数为15：原八份、本c13 partial一份、新c14六份。另五个c13名称只记录为未创建/应不存在，不能虚构成disabled。未来enable路径只能选择新c14当前inspector/delete用户，不能选择保留partial；关闭路径继续覆盖全部已知身份。一项非预期映射或enabled即失败，不将这条真实partial规则泛化为兼容空policy。

## 验收顺序

1. 当前受限transport的完整成功组合和ZIP/JSON/响应/身份/状态/重复/UNKNOWN反例；同镜像真实SDK合同必须覆盖两个旧API失败、单mapping导入、始终disabled及响应/sole-policy核验，独立评审后才进入Source。
2. 新源码提交与精确HEAD必要PR CI通过，再分配新停止资源；完整descriptor、network、mount、case、budget gates通过，新空预算只初始化一次。旧环境和预算只读核对。
3. 一次固定非删除准备：新六身份disabled创建/有限导入，15身份及17版本全量读取，三源Core前后不变、九新备份、分离clean/rehearsal恢复及实际业务重跑，全部关闭。任何失败保留并停止，不在原失败入口重跑。
4. 实际结果独立评审通过后才冻结candidate14及完整确认文本，再跑固定完整只读入口、离线核验及终复评；全部绑定和新source/HEAD CI分别报告。

源发送身份保持关闭，无真实DELETE、object PUT、cleanup、hold-release或bypass。#332保持Draft、不合并，#307保持OPEN；#323已按独立历史授权合并，不能重新声称其Draft。完整D1不提前验收。


## 当前收敛边界

本切片目标仅为剩余六份新候选的非删除准备和可核对的精确方案；本文件拥有bootstrap修复，`remaining_six_window.py`从Go常量生成唯一窗口，`remaining_sequence.py`拥有六份有限顺序，冻结确认文本拥有最终实际绑定。禁止并行维护另一套授权/队列/IAM语义。

当前阻断只包括：官方SDK/原生路径不可执行、固定范围或disabled/历史保留门禁缺失、实际完整预检/恢复不通过、源码/冻结证据漂移、必要CI失败。性能、通用IAM/多云/未来生产恢复不扩入本切片。当前已知生产adapter本地和隔离原生证据仍须与最终字节独立复评；Source/Restore未通过不得冻结。旧失败和 UNKNOWN 不得为获得绿色结果改写或退款。


最终 ParseQuery 拒绝畸形query的收紧落定后，隔离 native04 使用当前完整Go字节重新构建并实际通过两用户16请求生产 adapter 合同；终态SHA`8f8db644f2b23213d7d0b0e90ef93a7462c232f8eb036f7939a51146790ba108`，两个新CID均关闭/PID0，Source/DELETE/object PUT=0。native03仍仅拥有其原source snapshot，不伪称与当前逐字节相同。原API probe和native02/03/04共八个scratch caller均保留，旧90+失败c13的86+scratch8共184已知owner进入新代只读保留门禁。
