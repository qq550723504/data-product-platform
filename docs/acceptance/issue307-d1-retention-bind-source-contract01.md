# #307 D1：Docker Desktop bind 文件身份边界

本轮仅修复已合并 M `44c9bce432a5b5e393f3587964391e928a2639cc` 中 maintenance 宿主对 daemon bind Source 的解释错误。现有两份冷态 writer、五份 Linux 卷、密钥、binary、预算及原失败保留。#307 OPEN；不新建候选/窗口，不推送，不启动源环境或 writer，不初始化 ownership，不写保护或删除。

仓库已有 `docker_host_paths.normalize_source` 将 `/run/desktop/mnt/host/<drive>/...` 转换为宿主 drive 路径；retention 原路径绕过了它。不能把 daemon namespace 的字符串直接交给 Windows Path，也不能用更宽松的 `resolve()` 静默穿过链接。

修复复用该已有 namespace 转换，新增同一模块的 `bound_file_source`：

1. Source 必须是非空文本。Windows 只接受实际本地绝对 drive 文件，或已有的精确 Desktop drive namespace；后者只在 Windows 接受。拒绝空分量、`.`/`..`、NUL、drive-relative、UNC/device、ADS、尾随点/空格及混用反斜线的 Desktop 表示。
2. 在解析原路径每一级时复用 `evidence_indexes.unlinked_absolute` 检查 symlink/junction；先查原组件再 resolve。实际文件与批准宿主文件都必须存在、为 regular file 且 nlink=1。
3. 使用 Python/OS 的 `Path.samefile` 验证物理身份。大小写/8.3 表示只有确实指向该文件且无链接才接受；另一份内容相同的文件、hardlink、symlink、junction 都拒绝。namespace 解码本身不构成身份通过。
4. 原完整 Docker descriptor 的 raw Source 继续参与冻结 hash；不改写 inspect、不把宿主投影伪装成原始 descriptor。四项挂载集合、bind/volume 类型、只读性、volume identity、完整 CID 挂载集合、binary SHA、config、UID/安全参数、运行态与 ready/proof 门禁全部保留。

不新增第三方服务、路径映射配置、provider、队列或 Core 能力。复用已有 Docker CLI/namespace adapter 和 Python 标准文件身份 API，避免第二套并行转换逻辑。既有宽用途 `hostpath` 仍服务其他目录查询；本修复的 strict regular-file 验证用于 retention binary，不把目录查询等同于文件身份授权。

验收必须同时覆盖临时文件的成功/别名/链接/同内容副本及 rehashed 挂载扩张拒绝、完整两 owner 维护模型成功、normal/-O，以及现有两份实际 cold inspect Source 的只读比较。模型成功不能冒充原生 before/after、签名或真实写入；实际冷态比较也不能绕过 live topology 要求 Running/ready/ownership 的后继门禁。

本补丁尚未发布，其旧 M/main CI 仅证明修复基线，不证明补丁已通过 PR/main CI。未来若发布，仍需新精确 HEAD、发布后来源绑定与相应授权，不能使用旧候选/窗口/授权或改写已有冷态证据。

标准依据：[Docker bind mounts](https://docs.docker.com/engine/storage/bind-mounts/) 说明 Desktop daemon 位于 Linux VM，bind paths 通过 Desktop 与宿主共享；[Python 3.13 pathlib](https://docs.python.org/3.13/library/pathlib.html) 提供 samefile、is_junction 等实际文件系统 API。本实现不把 Linux/Windows 字符串相似或同一 SHA 当作物理文件身份。
