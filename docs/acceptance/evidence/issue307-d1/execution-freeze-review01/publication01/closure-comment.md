本切片收口：HEAD 22337ded634a87e7e761082a08b0a3635a3c2e4a。修复 v29 授权后 PR 分支仍被合入 main 的维护边界缺口；最终提交/精确 HEAD CI 后才启用 GitHub 原生 exact branch lock，锁后只追加回执，不再推进冻结 HEAD。授权必须新增 execution-binding SHA；完整只读入口、Host.check、H1 和发送拓扑签名共用 fresh HEAD/完整保护/精确 CI 来源核验。未知结果保持停止与锁定，seal 不依赖有效授权，不自动解锁。

独立复评 PASS、剩余当前切片阻断 0（B1/B2 已闭合）；normal/-O 各 33 项通过。独立 GitHub rehearsal 实际拒绝一次合法快进更新（422 locked branch），原分支/锁和原始回执保留。精确此 HEAD 的 pull_request CI39736086917 全部 13 项成功，未解决评审线程 0；这是 PR CI，不是 main CI。

有限 contract：docs/acceptance/issue307-d1-execution-freeze-boundary.md；矩阵：docs/acceptance/issue307-d1-execution-freeze-review01.md；独立报告：execution-freeze-review01/independent-final-review02.json（SHA256 651ef919f365399c100cd2ae5e9e0357e5cca38ca6aa371f2f8cb7c13f4a035f）。原 v29 失败、预算、pins 与证据保留，旧授权不复用。完整 D1 六份发送仍未运行；新候选准备另列，不将本修复 PASS 当新删除授权。#332 保持 Draft、不合并，#307 保持 OPEN。
