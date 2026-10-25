# #307 D1 candidate15/v25 非删除准备矩阵

状态：FROZEN_NOT_AUTHORIZED。唯一execution `9f9c67d7-8a09-45a8-897f-9e3077b60980`。人类选择的拟窗口2026-10-20新加坡14:00–14:40（UTC06:00–06:40），只读恢复上限08:40Z；仅用于准备，之后仍须新精确授权及单独调度授权。旧授权不复用。六目标P2/T1/T2/H2A/M1A/M1B、十一survivor、P1永久排除。

| 验收项 | 当前事实 | 剩余工作 |
| --- | --- | --- |
| 旧P2/T1/T2 closure、原17版本 | 实际独立PASS | 保留旧pins/预算/失败 |
| c13/c14失败 | 原始封存保留，未重跑；c14失败事实独立审查PASS，唯一blocker C14_HEAD_BINDING | c14 business2用户disabled/sole，4未创建用户须实际NoSuchUser |
| 当前policy head快照 | 实际只读PASS；Core前后等于旧独立基线 | 五实际successor取代旧snapshot引用，不Renew |
| 本地非删除回归 | Go270PASS/2SKIP；Python normal/-O各29PASS，含旧头/重算哈希/Core checkpoint不匹配/旧identity enable反例 | PG DSN及opt-in native未配置的SKIP不算实际验收 |
| 生成/完整预检/离线校验 | 共用实际checkpoint；独立复评PASS，0有效阻断；完整main额外positive+五反例normal/-O均PASS | 报告SHA7b8b5e63663d90acdc1d5b512c18349daf0f454ac904e65f99258dd47241325e；不替代native |
| 当前源码/CI | f6acb072db9881e7b1cfd72003d4ef616955947b，精确该HEAD PR CI39311610413 13/13 SUCCESS | event=pull_request；不是main CI |
| 旧270 owners/旧预算/备份 | 实际只读PASS，SHA2af0c9266d0af1a797462405832dd73248689ba03e985d7edc446acbdb2f9291 | 不启动或改旧资源 |
| 新完整非删除准备 | 实际三组PASS，92/92关闭、19profile撤销；terminal SHA921d7ed8b98974841c03de58efb7a53ef58198701236ead19765ea63d30087c4 | 独立06 PASS、0有效阻断，报告SHAa59f23dc692ea8f4adfccf529284633f58939c7a94d27f8de12ed2085b640728 |
| 本代实际Source/restore | 17版本、17known（16sole+1NONE）/9absent、三源Core前后、九新备份及clean/rehearsal分离恢复均PASS；业务试跑old multiset保留 | helper技术HTTP 268对start/obs；试跑另追加CostEvent 9+12+6，二者不混作一个总次数，amount NOT_ASSESSED |
| 本代86新owner/三预算 | 实际固定cold/init/check PASS，30permission false、三预算10001且空 | allocation SHA80cf4a96e73759116a8a62952888361a7db06547c4318404a6ce50e46f4f1682；旧预算/pins保留 |
| 冻结及离线校验 | 实际PASS；candidate 0ec2f2e03829905af97c9f71eafb108f7b3bf0899e7e741a0334d44520a5dc99，bindings d8a7075be2e8614f1af4fbaa159b45dc53430d19f06ea2920070b03c4e917da0；Go network-none离线PASS | source-freeze2111受核文件、原Git ZIP及12构建成员；不是2111份生产代码 |
| 宿主freeze失败与修复 | 第一轮Git Windows长路径失败、零输出证据及原空目录保留；仅第二次子进程追加core.longpaths=true后宿主冻结PASS | 不改源码/二进制/父环境/Git配置，不重跑Source/native；failure SHA51f40145c340f491b2c86770fdd53aceab85290f08f668fa316cb48fb01aa652 |
| 冻结后完整只读入口 | 实际PASS；complete SHAe2129d7abae7e3de937b801b952d31dd35516e096306f0311f89573358aff325；17版本/17known/9absent/三源Core/9新+9旧备份/30false/3空预算，source/helper结束关闭 | 101对技术只读请求（43/32/26），业务修改/enable/DELETE=0，amount NOT_ASSESSED；不能替代未来窗口fresh前置 |
| 精确确认/终审 | [完整确认文本](issue307-d1-candidate15-confirmation.md)已生成，102行机器模板原字节与6目标/11survivor一致；独立07 PASS、0有效阻断，SHAeaceae8610b25302b4ab83cc3441203c3b3f38a451d546bb6c5e644f39d9872b | 文档最终HEAD及对应PR CI另见追加[交付读回](evidence/issue307-d1/post-v22-policy-fix01/final-delivery-checkpoint01.json)，不使用f6ac源码CI替代 |
| 独立最终宿主读回 | 359唯一保留CID全部关闭、host私钥无worker挂载；真实sealed gate独立PASS | 6case/6recovery/30false/86新owner关闭/9新+9旧备份/3空预算全部匹配 |
| 归档与引用 | ZIP整体原SHA和各成员raw事实保留；1736成员仅CRLF与Git/LF不同，2111受核记录LF规范化相等 | 不声称ZIP成员与Git raw逐字节相同；50项[固定引用清单](evidence/issue307-d1/post-v22-policy-fix01/candidate15-approval-reference-index01.json)，SHA2974089b38aa9481ef06d529295481d6c8fd0cae53dcf0605d90dc9633a676be |
| 执行及调度 | NOT_AUTHORIZED / NOT_SCHEDULED / NOT_RUN；本轮DELETE=0 | 必须新精确授权，未来调度另需人类要求，完整D1不提前验收 |

权威[有限窗口手册](issue307-d1-v25-window-runbook.md)；[旧v24失败矩阵](issue307-d1-v24-preparation.md)。当前#332 OPEN Draft、不合并；#307 OPEN。#323已按历史授权合并。全部旧/新备份、失败、UNKNOWN、预算和pins保留；cleanup=0、hold-release=0、bypass=0；完整D1 NOT_ACCEPTED。

冻结packet Git anchor `dcf6f4f50939090144d8299ab841b3b30d8eff34`只供已知CID封存，不是发送授权。运行source仍固定f6ac；后续文档HEAD必须单独核CI，不把源码CI当最终HEAD或main CI。模板SHA492a83604d1c32589d5dbb3aa97d4f40a618e5a6bf86e9c0123c80fc67f3609a。

固定引用清单排除自身、当前矩阵和后续授权/CI回执以避免自引用。将来授权须另绑定该清单自身SHA及最终HEAD/CI交付读回；按引用批准保留实际人类原文，user_retyped_template=false。本矩阵中的未来链接不是执行/调度授权；最终读回将在提交CI成功后追加，未完成前不得声称其PASS。
