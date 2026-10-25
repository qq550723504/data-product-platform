# #307 D1 本地整合与受限发布边界

本轮只将已审来源 `be38a68ded0bd4267a2c7711b5c4bc8bd1012e60` 整合到已发布基线 `e54967e822847e97316e703ee413ffc1298cb627`，形成继承两者历史的新本地提交。旧来源、失败、预算、pins和历史授权均保留。解锁、推送、源启动、新候选、保护写入、业务对象删除、Ready和合并均不在本轮授权范围。

权威实现继续为 `issue307-d1-successor-dependency-contract.md` 及其固定完整只读入口；原退役契约和回执拥有原维护事实；既有 execution-freeze 契约拥有未来受限发布核验。本文件仅规定整合保全与本轮验收，不新增运行时或平行门禁。

整合必须同时满足：

- 以e549为第一父节点，be38为第二父节点；无force/reset/rebase或历史重写。
- 当前e549的全部已发布证据及retention源码/测试/宿主接线不回退；`fixture-runtime-artifact.json` 保持e549的Git对象与全部原字段，R仍为 `ecf40e6c7a9b3aa11166e6a56a5163a29a15691d`。C22/C23旧execution和过期窗口不因整合获得新授权。
- `prepare-six-candidate.py` 当前 `historical_gate()` 唯一调用新 `retirement_catalog.verify_history()`；旧C23与pre-C22函数仅保留历史解释，无错误fallback。当前profile/窗口/网络代际及retained_v32历史解释来自e549，不能回到be38的C22值。
- CI合并保留两边既有步骤及作业，新增successor normal/-O回归；旧retention用例不能丢失。
- 非删除回归覆盖原成功/反例、整合后prepare钩子成功及预算失败传播、现有宿主CI配方和保留期修复；完整现场入口只读且不启动源服务。独立评审只接受本轮实际整合字节，不复用be38评审充当整合评审。

阻断限于历史/数据/预算保全、安全授权、实际整合必需路径或CI回归失败。新候选、17版本源业务读取、保护延期、真实删除和完整D1验收为后续独立切片。本地验收不是远端精确HEAD CI；未来受限发布须另获绑定最终新HEAD的人类授权，并保持#332 Draft/unmerged、#307 OPEN。
