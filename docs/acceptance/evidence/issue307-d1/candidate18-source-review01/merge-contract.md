# candidate18/v28 有限准备与评审契约

目标：基于723e04d1的17版本fresh只读核验及三个v27旧dispatch的可信NOT_SENT闭合，准备六目标的新可执行候选。新execution为4fd5a7e7-779e-41d7-a35a-3bab7dd615b5，拟定2026-10-21新加坡22:00–22:40，恢复上限次日00:40。本轮没有删除授权或调度，不启用源发送身份，不运行sender/wire，不合并；#332 Draft，#307 OPEN。

唯一当前窗口由Go remaining_scope.go导出；case.CandidateUseUntil与Base.ExpiresAt统一为END，恢复END+2h。现有Asynq/Outbox、Command/官方MinIO SDK/策略ParseConfig/Equals、完整native reader/file-only checker/host attester/封存原语继续复用，不引入平行基础设施或领域迁移。

当前source业务Core精确绑定post-v27闭合03后的ffdf7501…；另两源仍用原冻结Core基线。aab0 policy checkpoint原字节、五head IDs和已过期期限保持原事实，不Renew，不以旧CoreSHA替代当前全关系/roles/ACL基线。

新六IAM身份只由本execution命名并以disabled创建；旧v27六身份进入retained，全部已知身份35、未创建身份9。新三预算卷归新execution、UID/GID10001、完整挂载集合、初始空；18旧预算、pins、失败/UNKNOWN/成本/备份均保留。旧历史集合固定为623CID与已绑定旧531控制/备份文件，不因新候选的目录创建而自扩张，也不漏验旧目录扩张。

非删除准备：实际源Core/native只做受限读与本代disabled IAM provisioning；源Core不运行业务Commands。新checkpoint/rehearsal Core、native、internal网络、队列和预算独立分配；业务Command只在新rehearsal Core且无DELETE凭据，GET/HEAD-only restore gateway。真实源和clean restore指纹不变，旧事实多重集保留；不重跑旧Processing/DirectData，不产生新业务输出。

冻结要求：原12构建成员及source/raw/LF/ZIP、reader/operator/owner/checker、Core dumps/globals/native档、18历史copy/17现存/6目标/11保留、exact copy/plan/hash、IAM/policies、gateway sealed/active、实际CID/image/args/完整mount/network、profile/window、预算及证据索引逐项绑定。所有permission=false、所有受管owner stopped；原授权和失败终态不覆盖，后续授权及调度须另存。

阻断限定：安全/权限扩大或secret泄漏；历史/预算/pins/成本事实丢失或改写；authoritative contracts冲突；上述必需准备/验收/冻结路径不可实现；核心并发/幂等破坏。非目标：真实六DELETE、H2/三副本真实mixed outcome验收、人类授权、未来调度、Ready/merge以及全D1提前验收。
