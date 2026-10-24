candidate22/v32 收敛检查点：剩余六份的非删除准备已收口。

最终 HEAD `8de3bc4c9f737c3c3e81fddb60491422ba6039f2`，精确该 HEAD 的 [PR CI run 39910327376](https://github.com/qq550723504/data-product-platform/actions/runs/39910327376) 13/13 SUCCESS，未解决评审线程 0。这是 pull_request CI；未合并，没有本代 main CI。

有限 contract 为 `issue307-d1-execution-freeze-boundary.md` 与 `issue307-d1-v32-preparation-contract02/03.md`。R/P 严格双制品、分配前 CI/子网核验、三组原生非删除备份恢复和副本业务重跑、17 版本完整只读入口、冻结包独立复评及同入口静态审计均通过，当前切片剩余有效 blocker=0。6 目标/11 survivor，P1 永久排除；1085 已知 CID 关闭、30 发送/root permission=false、30 旧预算和 3 独立新空预算保留，75 备份原字节保留。

原生 exact 规则 `BPR_kwDOUdQLBs4FE_7b` 已重新锁定；实际只有 lockBranch 变化，其他完整保护设置不变。锁后 execution-binding SHA256 `a76132a5abc97b16209cf95317376ce0a9cfb8b3a3d7c9c845fd0509071bc437`，公开/私有逐字节一致；后续锁后回执另存，不再推进 HEAD。execution `0e5cb8f3-46a9-4339-88e4-7154d6ebe13f`，候选窗口仅 2026-10-23 新加坡18:00–18:40。

本代删除授权 NOT_RECEIVED、定时续跑 NOT_CREATED，六份源发送/恢复仍 NOT_RUN，真实 DELETE=0，完整 D1 NOT_ACCEPTED。旧授权不复用；旧失败、已用预算、pins、UNKNOWN/attempts/成本保留，cleanup=0、hold-release=0、bypass=0。#332 保持 OPEN/Draft、不合并；#307 保持 OPEN。
