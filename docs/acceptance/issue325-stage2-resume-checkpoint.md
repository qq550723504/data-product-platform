# #325 第二阶段首项 — 原环境 restart/resume

2026-10-19。用户明确要求先恢复原环境，不重新 seed。基线 main / #326 merge 为 `1d1e8efd8217854cc3b09010f7e47b183fa824d6`。沿用本 chat 的已附加 worktree 和 `.data/trial`，分支 `codex/issue325-trial-stage2`；主 checkout 和 #307/#294 writer 不改动。

## 有限 contract 与复用

- 固定原 run `1b7e1949e8224ad8b870d28b96a3c6c2`、workspace `e7469d12-05bc-4427-b79a-356a49cfd13c`、Actor、6 个确切容器及 6 个 named volumes、network、image、端口、三 RAW 和现有事实；不会调用初始化、migration、prepare、seed、容器重建或镜像拉取。
- 复用现有 CLI 独占锁、Windows process identity、Docker exact-owner 校验、只读 durable storage checker、原 API/Worker/standalone Web 制品、`trial-prepare inspect` 的只读 SQL/native GET，以及既有 MinIO Go SDK / native STS。Docker start/stop 是成熟运行机制；不新增 supervisor、queue、workflow runtime 或 IAM。
- 原实现将 controller 和应用源文件混为一套指纹，不能无痕覆写旧指纹来绕过 drift。显式 resume 校验原 snapshot 与原 Git commit、原制品/配置/资源，再记录新的 controller binding。仅允许 `deploy/trial/**` 与新增 credentials-only helper 源码变化；其他应用/行业/fixture/migration 源码变化拒绝。本切片恢复原业务制品，业务制品升级与 B/C 另行绑定。
- 所有原 registry/source/artifact/config/credential 文件保留；每次实际续用写不可覆盖的原状态 snapshot、开始/完成或失败 observation。run/workspace 不变，新的 resume attempt 只标识宿主服务本次启动，不冒充新的业务 Operation。
- 过期的 Core STS 使用已有 native SDK 续发同 workspace、相同 staging GET / core GET+PUT 范围，写新私有文件和新 profile，保留旧 profile/credential。不更新 Authorization/Rights/Certification，不扩大权限。

## 顺序与失败边界

`resume` 仅接纳已知 STOPPED；先验证 commit/source、全部制品、配置、daemon/容器/卷/network 归属、旧进程已退出和原端口可用，然后持久化 resume attempt，启动确切原容器。原 Core/对象存储恢复后，在启动 API/Worker 前比较完整旧 SQL/native 对象事实；不一致或未知即拒绝成功。完成同范围凭据续用后，启动原宿主制品并再次核对事实及 HTTP 页面。已登记续用阶段的执行失败记录 UNKNOWN 并保留已记录 owner，允许显式 exact-owner stop，禁止自动重建/重 seed。

首次 journal 建立后、RESUMING 保存前中断，可能保留原 STOPPED registry 与部分 journal；当前拒绝自动采纳/覆盖，且尚未执行容器启动、凭据续发或宿主启动。此首次建立窗口的显式恢复列为 #325 非阻塞 follow-up，不声称所有写盘中断都会自动得到 UNKNOWN。本次已绑定原环境的正常 stop→resume 不依赖新增恢复协议。

RUNNING 的重复 `resume` / `start` 只校验和读取，不新增 attempt、凭据或业务事实。`restart` 是已知 RUNNING 的显式 stop→resume；未知状态不盲目重试。

## 写域与验收

允许写 `deploy/trial/**`、新增 `apps/platform/cmd/trial-resume/**` 和本主题验收文档/证据；Core/internal、migration、routing、worker main、#307/#294 及其他运行环境只读。只操作本次确切持有的 trial。无 DELETE、cleanup、down/prune、付费 Provider 或远程开放端口。

验证：无 registry/UNKNOWN/drift/foreign identity/端口冲突失败且零启动；重复续用不 seed；同权限 STS；原实际 run 的 resume、只读 SQL/native hash/manifest/版本关系、真实浏览器旧批次、重复 resume/start、stop→resume/restart，run/workspace/容器/卷不变且 Audit/Evidence/Cost/业务事实不增加。保留旧未知成本，不推断零 I/O。完整 B/C、认证、真实交付/撤权负例和 deferred 人工解释仍为后续，不据此关闭 #325。

先 Draft，独立复核及当前 HEAD CI 单列；本次没有新增合并或部署授权。
