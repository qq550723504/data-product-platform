# #307 D1 旧 dispatch 闭合的固定宿主入口

本追加合同落实 `issue307-d1-post-v22-dispatch-closure.md` 中的有限 observer 阶段。前一合同和初次只读审计记录其当时状态；#323 此后已按协调聊天中人类直接授权合并，实际 SHA `f22587d780bacc9c865b269fa20b1cb135034f5a`，对应 push/main CI `39250314443` 为 13/13 SUCCESS。新闭合代码保存在独立工作树，不属于该次合并。#307 仍 OPEN、完整 D1 NOT_ACCEPTED；本切片禁止真实 DELETE、预算退款和旧 pin 释放。

唯一入口是 `examples/lifecycle-d1/close-old-business-dispatches.py`。`prepare` 只构建新 observer、生成宿主独占签名私钥及关闭的零删除 profile，并创建三个 stopped owner：observer、固定 check-disabled、固定 disable-all。root helper 沿用原冻结 binary，只挂它必需的 root/candidate/IAM/policy 文件；不挂旧授权。observer 只挂 closure.json 和签名 proof.json，不挂 root 文件、宿主私钥、存储凭据或 budget。新 owner 使用 UID10001、只读 rootfs、cap-drop ALL、no-new-privileges、no restart、无发布端口，精确私有/二进制/输出三个 mount。必须先独立评审实际 manifest，再允许 `run`。

Docker owner 使用显式固定非空参数：observer 为 `post-v22-close`，root 为 `-action check-disabled/disable-all`；不依赖 Docker 将空 Cmd 表示成 null 或 []。失败准备01的 manifest、终态和更正诊断及三份stopped owner均保留；新的准备02在运行各阶段重新核对旧失败输入、关闭的profile、无proof和三个PID0，不能修改失败01使其冒充成功。

新非删除 profile 的 40 分钟期限仅在实际检查完成后开始，不是删除窗口，不构成删除授权。激活只允许 Enabled、NotBefore、ExpiresAt、EvidenceSHA256 四字段变化；其余字段和 source/input/binary/CID 必须匹配新 manifest。原 v22 窗口和只读期限均不延长、不复用。

执行前保存 77 个旧 owner、46 旧控制文件、8 只读 profile、九备份和三组旧预算与最终 P1 anchor 的精确一致性。39 个旧执行 helper 的 binary、operator/root 配置、case 配置和实际 CID 均核对。原冻结 case 的 Base.Enabled=true；终态封存合法地改为 false。只在内存中反向这一个已记录撤销字段，按原生成器序列化匹配 frozen case SHA；实际文件必须 false，并继续匹配终态保全 anchor，绝不恢复文件或启动旧 helper。旧 helper 的实际历史 UID 记录原值，不能宣称原 owner 都是 nonroot。

在任何 start 前核两源精确 CID/image、closed/PID0/noports/no-restart、唯一绑定 NetworkID，以及该网络 internal 且无活动 endpoint；然后仅启动绑定的 business Native 和 Core。先核完整 Core 无冲突原行，再由固定 check-disabled 取得官方 policy ParseConfig/Equals、四用户 disabled/单 policy/no groups 的实际请求和观察。技术请求逐项留证，amount 未知为 NOT_ASSESSED。停止 Native 后，再核全部旧 helper/source 关闭、三个 gateway 实际 mounted sealed SHA、旧预算/备份/控制文件和网络只含绑定 Core；原 P1 唯一 wire witness 验签和固定 SHA 保留。新鲜 IAM 完成≤45秒，签名 scope/time≤1分钟，host 私钥绝不挂 worker。

固定 observer 仅使用现有 RevokeDispositionApproval 和 RecordProviderObservation，授权能力只有这两项；无 StoragePort、队列消费者或 sender。P2/T1/T2 各追加旧 approval 撤销，再在原锁下核无 claim/observation/release/absence 后追加 NOT_SENT 和已有 dispatch release。原 attempt、dispatch、approval、pins、预算、UNKNOWN 和成本不改写。正常三组的完整事务增量为：revocation/observation/release 各3，command idempotency6，Audit/Evidence/EvidenceRelation/Outbox 各9；该数量已由全迁移真实 PostgreSQL Command 组合验收，不只由 mock 断言。

任何 owner 失败，wait timeout、日志失败和 inspect 失败分别保留原通道及其缺失状态。不能因一个通道不可读跳过其他通道。finally 首先撤销新 profile、停止 observer，随后逐项尽力保存 partial result、实际 Core rows/fingerprint/旧行 anchors。已提交部分结果不能回滚、重发或补造；不可读为 UNKNOWN_NOT_ASSESSED。若 Native 属于本轮 attempted-start 且仍运行或状态不可读，仍尝试固定 disable-all，不启动 Native、不 enable；之后逐个停止所有新 helper 及本轮 attempted source，失败一项继续其他项。关闭身份无 fresh 读回时为 UNCONFIRMED；最终保全差异或 Core 对账缺失均不得报 PASS。

离线成功/失败组合只证明控制流。实际 PG 组合只证明数据库/Command 语义。源码独立评审、manifest 独立评审及原环境执行分别留证，不能相互替代。实际三项可信闭合完成后才进入完整 17 版本只读核验；六目标新候选、execution、固定删除窗口和精确确认文本仍是后续门禁，不提前冻结或自动授权。
