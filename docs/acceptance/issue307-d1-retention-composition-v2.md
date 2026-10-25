# #307 D1：两份保留期延期的完整入口 V2

状态：本地非删除实现与评审；当前发布基线仍为 `f4e25a88f62da6bb14ed24da8b71b634be43f17a`。本文件不是源环境启动、保护写入、分支解锁、推送或删除授权。#332 保持锁定、Draft、不合并；#307 OPEN。旧方案、授权、失败、预算和 pins 保留。

## 有限目标与权威

只延期两份既有 GOVERNANCE 版本：`fixture/control-retention.csv@2606d71f-207b-42a7-b671-4b7f4ca4d194`、`fixture/m1-c.csv@81a8402e-390f-4247-9d34-552df5eadeb3`。完整 Copy/原期限/Native CID 由 `prepare-retention-renewal.py` 和 Go 内嵌模板共同约束；不是任意对象的通用保护管理器。

V1 把截止写死为 `2026-10-24T12:40Z`。该旧值已无法覆盖后续实际准备时间、40 分钟删除窗口及两小时恢复，因此不能继续用旧二进制/授权生成新业务方案。

V2 唯一日期来源为新冻结 bindings 的 `RetainUntil` 和 `NotAfter`：

- `NotBefore < NotAfter`，维护窗口至多 40 分钟；
- `NotAfter + 2h <= RetainUntil <= NotAfter + 72h`，日期精确到秒；
- root proposal、两行 `new_retain_until`、SDK `RetainUntilDate`、signed grant、reader、实际 receipt、checker expectation 必须一致；
- 实际 PUT 及紧随的读回使用该同一值，必须严格延长两份精确旧期限；
- **这只覆盖维护本身的恢复段。后继删除方案仍须独立检查它自己的真实 `RecoveryEnd <= RetainUntil`，不能凭本维护 PASS 宣称任意未来窗口有效。**

新 kind 为 `RETENTION_EXTENSION_EXACT_TWO_V2`，必须有新的 MaintenanceID、独立新预算、固定两个完整 owner CID、二进制/描述符/config/key/plan 绑定。旧 kind、旧执行和旧授权无效。没有默认新日期或自动平移/延期。

## 复用与边界

复用已有 `verify-six-native.py::collect`、真实 Core RR READ ONLY/fingerprint、SDK reader、file-only checker、`retirement_catalog.verify_history` 的保全快照/后继依赖/旧预算审计、`retention_maintenance` 拓扑及 Linux staging。使用现有 Docker CLI、MinIO SDK、cryptography Ed25519、标准库 O_EXCL/fsync；不新增 queue/workflow runtime、不在 Core 自建签名或存储。Temporal 等通用工作流不提供本次 exact identity/预算/授权事实，且仓库已有有限执行基础，不引入平行基础设施。

维护前、后均核对 17 个现存版本（7+6+4）、全 namespace、IAM disabled 语义、P1 exact absence、Core/roles/ACL、原内容/hold/版本不变。`before` 接受两份**精确旧到期值**；不调用删除专用“保护须覆盖 RecoveryEnd”的旧 gate。`after` 只接受这两行期限替换，全部其他行必须保持原样。双 phase 的实际 MaintenanceID、BindingsSHA、RetainUntil、MaintenanceEnd 都由实际 reader 写出，不能靠 host 改标签。

保全快照和旧预算审计只在所需资源冷态时运行；无关会话正常停启不属于后继实际依赖漂移。运行阶段只要求实际依赖，不能把维护快照中每个历史 State 永久冻结为运行条件。

## 固定完整顺序

入口 `python -B examples/lifecycle-d1/retention_composition.py`：

1. `check --packet <canonical-json> --packet-sha256 <exact>`：仅文件校验；无 Docker/源环境/provider。
2. `run` 除相同 packet 两参数，还必须提供 `--read-authority --read-human --write-authority --write-human`。入口不生成或签署人类授权。
3. 验证两份新 signed reference authority、原始人类 bytes（含 LF）、新 manifest/实际 bindings/key/binary/config/baseline 字节；实时 exact P HEAD PR CI、完整保护/同一 rule locked、Draft/Open。运行目录 `control/run` O_EXCL，已有目录/事实绝不重跑。
4. 冷态旧事实/实际依赖/旧预算审计；三组完整 `before` 只读核验并 finally-close。
5. 每组严格按 business-races → mixed：核对只属于本代的预算与挂载/目录、两个 writer 都关闭、权限 false、该组 output 尚空。物化**原文**和已签批准到既有 mutable 文件，先归档原字节，不从 host 合成人类文字。
6. 仅在 read+write 两授权均有效时启动固定 native 和 `-wait` writer。Go 先实际读到绑定 false，校验输入，再 O_EXCL/fsync 保存 `wait-ready.json`；host 不能用 Running 或 sleep 推断 ready。
7. ready 的 MaintenanceID/绑定/组/owner/descriptor/config/At 必须匹配，`actual StartedAt <= ready.At <= now` 且一分内。实际拓扑、Linux UID/mode/nlink、完整 mount/CID 集合、预算、前序 immutable 成功事实通过后才签一分内 proof；再核授权/截止，才 Enabled=true。
8. writer 的持久 invocation/slot 先于任何真实 SDK PUT。每对象最多一次、共两次，SDK 无隐式重试；每次请求前重新核完整实际 bindings/proposal/授权/proof/预算。执行后关权限、信任 exit0/PID0 和真实 exact readback，第一份未确认绝不启用第二份。UNKNOWN、超时、成本/attempt、已用预算保留，不 rollback、不重发。
9. 三组完整 `after` 只读核验，finally-close；封存两个 writer、三组 Core/native、原 business Redis；冷态复核旧历史/必要依赖/预算。只有所有阶段及闭合都通过才报告完整 PASS。

读授权独立 kind 为 `EXACT_TWO_RETENTION_READONLY_AUTHORITY_V2`，精确绑定 manifest、MaintenanceID、bindings、实际原文 hash、SourceStart=true、Phases=[before,after]、ProtectionWrites=0、ProviderDeletes=0，最早维护前 15 分钟、最晚维护后两小时。写授权至多维护 40 分钟、max1/max2，绑定同一 RetainUntil。读期限不授予写入、延长维护或重新启用任何删除身份。

## 包与封存准备

新 P 精确 CI 及重新锁定通过后，才允许另行准备实际包；本实现验收不分配真实 MaintenanceID/窗口/owner/预算或签操作授权。实际包需冻结：

- V2 binding（含 RetainUntil）与新编译 trusted-public-key writer；三个 reader/checker/主机入口属于审过的同一 P/R；
- `retention_maintenance` host 的两组 Linux private/output + 独立共享预算 metadata、完整固定挂载集合，旧卷/预算绝不改造；预算初始只有本代 `ownership.json`；
- 三组原固定 source descriptor、root read config、实际 Core checkpoint、原 frozen/present inventory、精确 before/after effective inventory、完整 checker baseline；文件引用统一 path/SHA/bytes、nlink1、无 symlink/junction；
- host 私钥保留在 control 中，不进入任何 reader/writer/output/budget 挂载；两份新实际人类 reference authority 单独留证；
- 独立 containment anchor：schema、MaintenanceID、bindings SHA、host public key、两份 permission 路径、九个固定关闭 CID（两个 writer + 六个 Core/native + 原 Redis），以及六个只读 owner 的固定 nonce/name/native/profile/意图/登记/关闭回执路径。AuditID 由 MaintenanceID + phase/group 的 UUIDv5 确定。它自身的 SHA 单独写入批准文本/运维回执。
- 每个动态 reader 在 create 前保存签名意图，实际 create/inspect/mount 校验后、start 前保存签名 CID 登记。reader 不挂 host 私钥。封存独立验证这些登记后尝试固定 CID；缺登记、创建结果未知、关闭回执不完整或 reader stop 未确认，整轮必须 UNCONFIRMED，仍继续其余所有已知关闭。不得只关九个固定 CID 就声称 reader 也已关闭。

发生任何非预期失败，不再启用下一 writer。即使主 packet/config/binding 损坏，仍可用：

```text
python -B examples/lifecycle-d1/retention_composition.py seal \
  --anchor <approved-anchor.json> --anchor-sha256 <approved-sha> \
  --output <new-exclusive-evidence-directory>
```

seal 仅按钉住清单尝试 disabled 和所有已知 CID stop；不依赖当前运行包/日期/写授权，不启动任何源或 provider client。归档失败仍尝试关闭，某项 inspect/stop 失败继续其他项；归档不完整、未知 owner、读回不完整必须 UNCONFIRMED。原字节及 UNKNOWN/部分成功事实不覆盖，不退款、不重建预算、不删证据。

## 验收界限

完整模型回归不冒充真实源环境保护写入。真实两份延期、17 版本 before/after 原生验收、后继六份候选仍分别 NOT_RUN / NOT_AUTHORIZED。新源码发布需要绑定新 P/R 的另一次受限发布授权；`f4e25a88` 的旧发布授权已经用完。
