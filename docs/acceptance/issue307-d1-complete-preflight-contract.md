# #307 D1 完整预检有限修复契约

目标：将 case、owner topology、internal network、persistent budget 的必需前置全部收进 `verify-remaining-native.py` 的完整 CLI，在任何源服务启动前完成；窗口执行不再补写这些核验。v21 的失败脚本、授权、停止和封存证据保留原字节，本 execution 不恢复。

唯一原始 Docker 调用复用 `prepare-remaining-runner.py`：`call` / `docker` 返回 bytes，`docker_text` 只在显式文本边界严格 UTF-8 解码；str 或其他类型不得静默编码、自动兼容或继续执行。JSON 可直接解析 bytes。复用 subprocess、现有 Docker CLI、原生 reader、官方 MinIO policy checker、既有 finally-close；不新增进程/队列/策略基础设施。

完整 CLI 的顺序：路径规范化及实际 profile 跨字段/SHA门禁 → binary/候选锚点与新 O_EXCL 证据目录 → 全部关闭 owner/权限/备份 → 七 case 的固定 CID/image/command/mount/netns/noports/no-root/no-socket/norestart → 七独立只读 recovery → 三 internal network → 三持久 budget 的 execution owner、实际完整挂载 CID 集合与只读复制空目录 → 三组 Core/native/库存/IAM 采集、官方语义 aggregation、逐组 finally-close → 完整成功回执。任何前置失败保留 failure，不启动源、不启用身份、不创建发送授权、不写 complete 成功。

已终止旧候选只做明确命名的 retained-owner/backup closure；其已撤销 profile 不得当当前可执行 profile。该历史关闭辅助不是完整预检通过证明。新候选冻结后的独立完整 CLI 必须走上述全部路径，且不使用旧授权。

回归必须从完整 `main` 进入：成功组合实际执行固定宿主 gate 与三组 collect/aggregate/finally-close（Docker recording double + 实际 Go file checker）；错误输出类型、错误 budget owner、额外或缺失挂载者、非空 budget 在任何 source start 前拒绝并保留失败。另验证统一 wrapper 的 bytes/str/UTF-8 边界，保持已有 window、policy 排序变化/扩权及关闭故障回归。

非删除原生验收需在新的未授权准备环境中，使用同一完整 CLI 验证实际配置、39 owner、9备份、3 network/budget、18版本、完整disabled IAM、Core前后不变及全部关闭。mock/replay、局部 collector、历史只读通过和终止后的预算观察分别报告，不能代替这次完整原生成功。修复及独立评审先收敛，再确定新冻结窗口；未选定窗口时不虚构新的可执行候选或精确授权文本。

非目标：真实 DELETE、旧 execution 恢复、旧 profile 解撤销、hold release、bypass、cleanup、合并、第二套 workflow/IAM。#323保持Draft，#307保持OPEN。阻断仅限本契约必需路径、权限/保留边界、历史证据丢失和CI失败。
