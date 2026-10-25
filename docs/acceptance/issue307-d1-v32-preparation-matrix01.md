# #307 D1 candidate22/v32 准备进度

本矩阵记录2026年10月23日的新准备事实，旧candidate21的冻结文档、核对PASS和历史字段不回写。

| 项目 | 当前事实 |
| --- | --- |
| 新窗口 | 人类已选择2026-10-23新加坡18:00–18:40；UTC10:00–10:40，只读恢复最晚12:40Z |
| 新execution | `0e5cb8f3-46a9-4339-88e4-7154d6ebe13f`；candidate22/v32仅计划，未分配、未冻结、未授权、未调度 |
| 旧candidate21 | 窗口及恢复期已过，删除/调度授权均未收到；新追加终态`EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED`，private/public同字节 |
| 旧代闭合 | 原固定cold gate通过；996个已知CID停止/PID0/noports/norestart；30permission完整冻结字段false；3个该代预算仍空；27个更早预算及66份备份保留 |
| 终态恢复拒绝 | PASS；原allocation_state()在读取allocation前拒绝旧代继续，未放宽门禁 |
| 有限现有回归 | 正常/-O各21项PASS：execution_freeze、remaining_six_window、preallocation_ci；不冒充新代运行接线验收 |
| 当前PR | GitHub实时确认HEAD仍`f4f27c789f53b6c45910f55f83bee6cabfdee4f6`、OPEN/Draft/unmerged，该HEAD原run39773563527的13个jobs仍SUCCESS；不是新source CI或main CI |
| 分支保护当前fresh读取 | UNCONFIRMED；本地gh TLS超时，connector管理GET为403 Resource not accessible by integration，未执行任何保护修改 |
| 分支维护许可 | NOT_RECEIVED；拟只将exact rule `BPR_kwDOUdQLBs4FE_7b`的lockBranch置false，保留全部其他保护，须人类保护owner明确授权及fresh完整保护核对 |
| 本地准备 | 独立本地worktree/branch已建立；旧锁定HEAD和tracked执行字节未修改；新execution/窗口/维护proposal及有限contract已记录 |
| 后续source/CI/预算/原生/冻结 | NOT_RUN；受控维护及精确新source PR CI先于资源分配，不使用旧CI替代 |
| 完整精确确认/独立新代评审 | NOT_READY / NOT_RUN；不把计划写成完整冻结方案 |
| 本轮副作用 | Source start=0、enable=0、DELETE=0、branch unlock=0、push=0、new budget allocation=0；旧授权/预算/pins/失败证据保留 |
| 完整D1 | NOT_ACCEPTED；#332保持Draft不合并，#307保持OPEN |

有限边界：[准备contract](issue307-d1-v32-preparation-contract.md)。可核对的分支维护动作：[精确proposal](evidence/issue307-d1/candidate22-planning01/protection-owner-maintenance-proposal01.json)。此维护proposal没有删除或调度授权。
