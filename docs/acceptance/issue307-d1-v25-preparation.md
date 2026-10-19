# #307 D1 candidate15/v25 非删除准备矩阵

状态：PREPARING_NOT_AUTHORIZED。唯一execution `9f9c67d7-8a09-45a8-897f-9e3077b60980`。人类选择的拟窗口2026-10-20新加坡14:00–14:40（UTC06:00–06:40），只读恢复上限08:40Z；仅用于准备，之后仍须新精确授权及单独调度授权。旧授权不复用。六目标P2/T1/T2/H2A/M1A/M1B、十一survivor、P1永久排除。

| 验收项 | 当前事实 | 剩余工作 |
| --- | --- | --- |
| 旧P2/T1/T2 closure、原17版本 | 实际独立PASS | 保留旧pins/预算/失败 |
| c13/c14失败 | 原始封存保留，未重跑；c14失败事实独立审查PASS，唯一blocker C14_HEAD_BINDING | c14 business2用户disabled/sole，4未创建用户须实际NoSuchUser |
| 当前policy head快照 | 实际只读PASS；Core前后等于旧独立基线 | 五实际successor取代旧snapshot引用，不Renew |
| 本地非删除回归 | Go270PASS/2SKIP；Python normal/-O各29PASS，含旧头/重算哈希/Core checkpoint不匹配/旧identity enable反例 | PG DSN及opt-in native未配置的SKIP不算实际验收 |
| 生成/完整预检/离线校验 | 共用实际checkpoint，旧head/foreign/missing/extra及重算哈希反例已补 | 独立评审及精确HEAD PR CI |
| 旧270 owners/旧预算/备份 | 实际只读PASS，SHA2af0c9266d0af1a797462405832dd73248689ba03e985d7edc446acbdb2f9291 | 不启动或改旧资源 |
| 新完整非删除准备 | NOT_RUN | 全新allocation/budget；17known（16sole+1NONE）/9absent、17版本、三组Core/九新备份/分离restore/rehearsal |
| 冻结/完整只读入口 | NOT_RUN | 三组实际PASS及独立结果后冻结，再实际完整入口和独立评审 |
| 精确确认/执行 | NOT_READY / NOT_AUTHORIZED / NOT_RUN | 未调度；本轮DELETE=0 |

权威[有限窗口手册](issue307-d1-v25-window-runbook.md)；[旧v24失败矩阵](issue307-d1-v24-preparation.md)。当前#332 OPEN Draft、不合并；#307 OPEN。#323已按历史授权合并。全部旧/新备份、失败、UNKNOWN、预算和pins保留；cleanup=0、hold-release=0、bypass=0；完整D1 NOT_ACCEPTED。
