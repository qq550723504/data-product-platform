# #307 D1 candidate14/v24 非删除准备：失败保留

execution `afc0e66c-535b-426e-a937-964d0c9ca0d6`；源码 d9bf01248a8d9a0e3c15ff7548fb62d141f74bf7，PR CI39303939210精确该HEAD 13/13 SUCCESS。未冻结、未授权、未调度，本代真实DELETE=0。原窗口准备描述不是授权，失败后不重跑本execution。

固定入口在business独立rehearsal中被正确的expected-head门禁拒绝。根因是生成器复制candidate12的旧五头，漏掉v22已提交的六个immutable Renew；新的只读Core快照证明五头均为旧头的直接successor，Core前后仍等于已独立验收基线。原错误在trial Docker logs的stdout，stderr为空。下一代统一绑定实际当前checkpoint，不改旧快照、不Renew、不放宽门禁。

| 验收项 | 实际结果及边界 |
| --- | --- |
| 旧P2/T1/T2 dispatch及原17版本 | 已实际独立PASS，原事实保留，不代表本代三组完成 |
| 新86CID/15卷/3网、30权限关闭、3新预算 | 实际冷态/预算独立PASS；三预算10001:10001且空，旧预算未改 |
| 本地/生产adapter/host源码 | 旧d9bf范围内已独立PASS，不能覆盖此次实际业务副本失败 |
| business disabled bootstrap | 16真实IAM调用成功；新2用户disabled/sole/no groups，Source身份从未启用 |
| business Source只读 | 7版本bytes/SHA/hold/retention/inventory、P1 absence；39真实只读调用；Core前后完全相等 |
| business备份及初始restore | 3实际备份保留；clean与rehearsal初始Core等于Source，native full-file compare通过；不代表完整restore SDK/业务试跑通过 |
| business trial | FAILED；restore-only inspector临时启用后已fresh confirmed disabled，技术调用enable4/disable2；无Source identity enable |
| business immutable-after/完整RESTORE SDK | NOT_RUN；不能声称试跑后old multiset已验证 |
| late/mixed源与恢复 | NOT_RUN；该代4用户不存在，不能冒称disabled |
| 最终物理封存 | 92/92关闭/PID0，19maintenance/trial撤销，30permission false；三空预算及旧九备份保留 |
| 候选/精确确认/执行 | NOT_FROZEN / NOT_AUTHORIZED / NOT_RUN，完整D1 NOT_ACCEPTED |

失败terminal SHA256 `680dbaeeca4f6e8bfa7097860994d5fee7c7b96ac71617cc6a4542aab94194f7`；原450成员索引保留。技术attempt实际61（provision16+Source39+restore inspector6），amount NOT_ASSESSED；不把技术调用算作DELETE。

只读当前policy checkpoint：[post-v22-policy-heads01](evidence/issue307-d1/post-v22-policy-heads01/policy-heads.json)，SHA256 `aab0e61530bee63429a15ce635880a6d1db286cb744767f7d9fdb50082313df8`，Core SHA256 `74e45ae37d0dd4e998e461624dcddfffa80116a08cf3234a7e00dd0ad1fce6f8`。新候选准备见[v25矩阵](issue307-d1-v25-preparation.md)。全部失败、旧预算、pins、备份保留；cleanup=0、hold-release=0、bypass=0。#332 Draft、不合并；#307 OPEN；#323历史已合并事实不回写。
