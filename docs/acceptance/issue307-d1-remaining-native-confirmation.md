# #307 D1 新原生 fixture：v16 精确绑定与确认前条件

2026-10-16。**candidate06-preparation/v16 已冻结，发送预算=0。此页是准备方案，当前不能据此授权真实删除。** #323 保持 Draft，#307 保持 OPEN。

已核验的有限设计见 [剩余 D1 contract](issue307-d1-remaining-contract.md)。本轮创建了三组全新 native fixture；没有调用真实 DELETE、hold-release、bypass 或 cleanup，没有启动旧 5d/旧 LC-13，也没有复用 candidate05/v15 授权。

## 1. 候选及 source/binary

- 准备候选：`candidate06-preparation/v16`。
- 新 operator execution：`fc9d632a-45fc-445a-9652-211db29694b3`。这是准备批次身份；不是业务 Processing Execution，也不是发送授权。
- source：`de5f7dd2683cc5b5ed3a68528364d6fe59836f10`。
- 768 个 source 文件：原字节 + 仅 CRLF→LF 的双 SHA256，见 [冻结清单](evidence/issue307-d1/remaining-native01/source-freeze.json)，清单 SHA256 `80a32c8fe5b552d4e6090baf9d3799a19e89738dba58eb0a6b16b42d1485d598`。
- 完整候选、容器/network IDs、每个计划、目标及 survivor 的机器可核对绑定见 [candidate06-preparation.json](evidence/issue307-d1/remaining-native01/candidate06-preparation.json)。
- prospective target digest：`88f9bc751eedf6b0b7f59f228e14fa64c7399817d0b7a1a5a45ad3f1cbcff3aa`。
- 全部 16 版本 inventory digest：`c629e8bab52461c493c164e91845a4d1b15450efc0ffb40656d3f543f970e650`。
- Linux/amd64，Go1.26.5，CGO_ENABLED=0。下列 sender/recovery 仅为复用基线的 provenance references；新 case profile **UNSUPPORTED**，不可发送：
  - `reference-sender` SHA256 `4614fe539f2a41a39fa3d07bf0adf10f832fcc7dff04abf6e9f538617a21b4cd`。
  - `reference-recovery` SHA256 `dfb2b5e3c76164868bdb8950dbd153e5e8ebf0ddf4a446b36c92d7f1be1c10c3`。

这不是把旧 sender 更换 execution 后当作新 runner。新的 Asynq/NATIVE / authenticated DirectData、wire pause/witness、未知结果恢复顺序和七目标 sender 仍须接线、回归、独立复评，再生成新的可执行候选和用户确认文本。新 binary/source/身份/窗口的任何变化都必须重新冻结；本页 execution 及旧授权都不会自动变成新发送权限。

## 2. 三组精确环境 / Core / IAM / gateway

三组 project/network 均为新 allocation；每组使用 `dpp-lifecycle307-online` 和 `dpp-lifecycle307-quarantine` 两个固定 bucket。internal 网络、无 host ports，Core/object/restore 均为 retained named volumes。源库、恢复库、原生服务、只读 verifier/helpers 均 stopped/PID0；旧失败 helpers 同样保留。

统一缓存镜像：
- PostgreSQL `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`。
- Native MinIO `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`；native binary `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`。
- Caddy `sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f`。
- SDK `minio-go/v7 v7.0.83`、`madmin-go/v3 v3.0.94`。使用原生 API/Adapter；没有新 policy/queue/storage runtime。

### business-races

- project `dpp-lifecycle307-808a6960`，run `808a6960-6c36-4200-ae98-f18a68472fe5`。
- workspace `a4aa2689-6e8f-4180-a567-dfb875407eac`。
- provider `cc068c8c-4f00-44cd-a825-377631253d59`；incarnation `d5f220ca-6133-42a0-a31c-1cf717d05274`。
- network `6354fa508f7adb9ac66fc895e539786a5575e0b3754d9fdb2677d0557ab7c942`；subnet `10.234.166.0/24`。
- manifest SHA256 `2fd4356702a7903af53d4f1f6f8dc6c40372bed11b7069bb32beb130a5077c35`；checkpoint SHA256 `c0034f56441f7179a41eea189608e97188a7bbafe20cc3d330632b57b9298736`。
- prepare binary `ed6b6ccb27f52fc46f7576a4c5ea5428a7502ca2028ec357b4d01839689d3568`，source `1dc9a2e1b08bc80e458f4ef8d05888dccfcd4327`；migrate binary `c81031b224cf6fa33380850e95d03823ad7d26f2d2ab297c35607f9423bc9b78`。
- GET/HEAD-only verifier binary `687bbe2172368cbc9cf1690cbaab0f08dfc20ab69d9dc3de2aba6f8653eef166`。
- Core checkpoint `dpp-lifecycle307-808a6960-checkpoint` / `562fc35651295d65d18f81c0ae85f9404cb9377cbbcc37a82b2a83358e7abfd4`，volume `dpp-lifecycle307-808a6960-checkpoint-data`；194 表 / 372 行，完整表 row/hash、全部角色 hash、table/schema/function/default ACL 匹配。
- dump `17e173195e543359870b3fb73a69fe887775390ed87f64cb9557a285648d7ef6`；globals `a2e3163d862b1299efd06ecb0903cdfe8f08f1a5876030a03c5bc9737e31c2d6`。
- native archive `1bfc047b61d2f2a4a3b8140d3d0cc870804cf8131b07a3359e8c58d0a4dfc9da`；恢复 volume `dpp-lifecycle307-808a6960-restored-objects`；exact GET/full inventory/retention/hold/IAM 和文件 SHA256 比较 PASS。
- IAM delete identity `delete808a6960`，policy `lifecycle307-remaining-delete-808a6960-6c36-4200-ae98-f18a68472fe5`，SHA256 `3f1c10f0070d5cd2658d2c5b3422ae95e889220c9622ad5047f97fbf05f74d99`；delete/inspection 均 `disabled`。reader/producer 尚未 provision，root 仅私有 bootstrap/管理 probe。
- active gateway draft `b0d31b81143a611b60496e0fd8b98b40b877f1dd31db48b0cd5e687eb109f47e`（仅 validate，未安装）；已安装 sealed/DENY_ALL gateway `6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad`。
- [原生核验及回执](evidence/issue307-d1/remaining-native01/business-races/checkpoint.json)。

| Case | plan | scope hash | required targets / status |
| --- | --- | --- | --- |
| P1 | `621893ed-16fb-4f21-a6c6-2711d349107c` | `a852eb63484945a92996667f6ccee9d39e6dc0dbf20c864ff62310a383e3a0b8` | 1 / FINALIZED |
| P2 | `bce1b1b1-d6e2-44fd-8b87-171142350ab9` | `d4758d9b64003d92aca60290fee1d2f22e7a857d2fe2be513d4a1ea2755ce6a3` | 1 / FINALIZED |
| T1 | `247a2ca9-d2f3-4384-bcee-b10add497175` | `7c4daf909a375bc21fcf8dd09b14d99960ae1f919ce138eb517ac0593faf1c1c` | 1 / FINALIZED |
| T2 | `531c7b96-173f-4028-be97-2de9ed6c72f2` | `84fbafb913dba44e5ceb43d670203e55f0244fad62a2dc2fd9ebb10b87e0cab5` | 1 / FINALIZED |

### late-hold

- project `dpp-lifecycle307-754ea8f0`，run `754ea8f0-6ffc-4cdc-ac28-40fe8b590a1b`。
- workspace `64f71fd8-def6-46a9-a314-f4645b1595b1`。
- provider `ab8ba084-1768-401a-9f27-fd4dc0c27dad`；incarnation `9d5c2daf-40be-4155-811a-879f24ad0314`。
- network `e27a29ee55732447bff932d15bcb83339784089c00f03b9139232417fe1a8847`；subnet `10.234.167.0/24`。
- manifest SHA256 `fa787f45239d7b8d02367fff7827fbb0a04613f55fdaf5e965cb66d0916b2a80`；checkpoint SHA256 `15315a7a75ee48ba568b6bfc7bf3def8981b6912e30113b8cf1f9bcd2bbc1794`。
- prepare binary `49a66c5a5bb4ed5d64ebd35fe6cd171f85d48ba1987425e2b8e1e9009745211f`，source `dde201728d6ba536109da674d58529e8ffd4f6f6`；migrate binary `c81031b224cf6fa33380850e95d03823ad7d26f2d2ab297c35607f9423bc9b78`。
- GET/HEAD-only verifier binary `49a66c5a5bb4ed5d64ebd35fe6cd171f85d48ba1987425e2b8e1e9009745211f`。
- Core checkpoint `dpp-lifecycle307-754ea8f0-checkpoint` / `73fd141387770a972cea41fcd26161a570bbb1b2d4879b228d6d83a11b112cc7`，volume `dpp-lifecycle307-754ea8f0-checkpoint-data`；194 表 / 357 行，完整表 row/hash、全部角色 hash、table/schema/function/default ACL 匹配。
- dump `5ab181094c6153f7f0ec7546675eae211f5c2a1ffb800e91b4c2753ed55a8388`；globals `34c284dc7d999bf106459bf2cf694062e40222ce85e20a3c9916843b26ed253b`。
- native archive `8c72360ddad639fe5c2393d8ae35cb472fbd7223e485edb599035188274b6708`；恢复 volume `dpp-lifecycle307-754ea8f0-restored-objects`；exact GET/full inventory/retention/hold/IAM 和文件 SHA256 比较 PASS。
- IAM delete identity `delete754ea8f0`，policy `lifecycle307-remaining-delete-754ea8f0-6ffc-4cdc-ac28-40fe8b590a1b`，SHA256 `a52b6672772f35c043f677e9d41541e4bfef2e2cf92dcc38221e71732f6d42cb`；delete/inspection 均 `disabled`。reader/producer 尚未 provision，root 仅私有 bootstrap/管理 probe。
- active gateway draft `a3aca5607f3eb8dd37bd0459461055b2e8161275a5c27564e741477efb4b7677`（仅 validate，未安装）；已安装 sealed/DENY_ALL gateway `6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad`。
- [原生核验及回执](evidence/issue307-d1/remaining-native01/late-hold/checkpoint.json)。

| Case | plan | scope hash | required targets / status |
| --- | --- | --- | --- |
| H0 | `b1fe2a92-0a3e-400b-a0ca-338c120ad74e` | `67ff4eb1ac6dd477bf51a398745272ed47b68cfc6bb5aaa82fd7d3c3cad7e05a` | 1 / FINALIZED |
| H1 | `620d0676-f3fa-42d1-b3bf-f34e683de43c` | `89adf8f6854dfe03702ddc44daff92af79b39de2d4228d640ac765a6606738e2` | 2 / FINALIZED |
| H2 | `d78a9339-0968-45c9-bab6-bda86810ec33` | `205cff03fcbaf25ff4b5d3ad690e5261a8a45e626180aa0782f22e4e249bbffd` | 2 / FINALIZED |

### mixed

- project `dpp-lifecycle307-dfda43b4`，run `dfda43b4-4419-433d-959b-c617556953fd`。
- workspace `91f84817-9f5a-4654-a793-92e51f1fb7a6`。
- provider `01845fb6-7cc2-4010-8f2a-316576391e67`；incarnation `2b760525-880f-468a-a027-a6851cd06487`。
- network `77f72e9ef0781527a7e8e5ce4a6cd8d36018e527a9ea853ae72d5f8fff9ea0eb`；subnet `10.234.168.0/24`。
- manifest SHA256 `7fd71687b4ed7151a3c37a789c6fd5beebea2bd1d61c452c0051cc7a1a08e972`；checkpoint SHA256 `84c741ae7b6c12c61b77aa101dd538214e44bc760790bc17071413d719487d74`。
- prepare binary `49a66c5a5bb4ed5d64ebd35fe6cd171f85d48ba1987425e2b8e1e9009745211f`，source `dde201728d6ba536109da674d58529e8ffd4f6f6`；migrate binary `c81031b224cf6fa33380850e95d03823ad7d26f2d2ab297c35607f9423bc9b78`。
- GET/HEAD-only verifier binary `49a66c5a5bb4ed5d64ebd35fe6cd171f85d48ba1987425e2b8e1e9009745211f`。
- Core checkpoint `dpp-lifecycle307-dfda43b4-checkpoint` / `bb0b3ff06c5105322fd9406b0f0ca98fcf0824af96b771d7e0acb6d6f61adebe`，volume `dpp-lifecycle307-dfda43b4-checkpoint-data`；194 表 / 259 行，完整表 row/hash、全部角色 hash、table/schema/function/default ACL 匹配。
- dump `d53f19e539888b2f65dc0e8ae2ca51422a3e63e98480aff5593ef47b0c3c0d38`；globals `ac037cc93683698cf9ecb0288216d4143d2c8b9ce3ce8036bbb4594d50492b5b`。
- native archive `f3df9d1cd179f11fff5e0355844df6281d0eef1b28bb1f0af7ab499bf5891c81`；恢复 volume `dpp-lifecycle307-dfda43b4-restored-objects`；exact GET/full inventory/retention/hold/IAM 和文件 SHA256 比较 PASS。
- IAM delete identity `deletedfda43b4`，policy `lifecycle307-remaining-delete-dfda43b4-4419-433d-959b-c617556953fd`，SHA256 `14fdedae1ceb249d887461a73fd7f7bdc5a103b5956f01d3c0e8862db1279f17`；delete/inspection 均 `disabled`。reader/producer 尚未 provision，root 仅私有 bootstrap/管理 probe。
- active gateway draft `e744dce61a6286b7f7e311400f2360616b7f2ff45ce0a97d2c54dbfb0587eed7`（仅 validate，未安装）；已安装 sealed/DENY_ALL gateway `6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad`。
- [原生核验及回执](evidence/issue307-d1/remaining-native01/mixed/checkpoint.json)。

| Case | plan | scope hash | required targets / status |
| --- | --- | --- | --- |
| M1 | `c2596491-a620-4cda-83d1-4697c64e27f7` | `8e4380ab68e62e934dae7609b2b4b1711e8c66d0fa2a5a742d0ce6cf059f3a75` | 3 / FINALIZED |

每个 plan 的完整 required membership 不等于允许发送的子集：H1 全部 0，H2-B 0，M1-C 0；completion 必须仍按全部 frozen required targets 判断。平台 approval/dispatch/provider attempt/pin 当前均 0，限制/processing/DirectData 竞态尚未运行。

## 3. 精确版本与保护状态

未来可能申请最多七份源版本、每份最多一次，总窗口最多40分钟；**本轮全部为0**。其余九份属于 survivor，包括未设 native lock 的 H0/H1/H2-B，不能因“无锁”扩大范围。native retention 仅证明保护读回，不代表已运行 DELETE403。恢复副本、全部备份、失败环境中的另外12份版本、其他环境均排除。

| group / case | exact namespace/key@version | copy / DatasetVersion | SHA256 / bytes | native protection / prospective budget |
| --- | --- | --- | --- | --- |
| business-races / P1 | `dpp-lifecycle307-online/fixture/p1.csv@fed0102b-bc8b-4f47-b9e2-fae8d5c563ce` | copy `5a4c9789-3e67-4e54-ab58-128d111cbd65`<br>DV `76f7f44e-4a2a-46c6-a6dc-a529f9ade834` | `722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0` / 232 | NONE / 1 |
| business-races / P2 | `dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302` | copy `6fabf3b5-76b8-4245-8f5d-01e64f610146`<br>DV `4219373a-07a0-4c66-a666-305675304b14` | `722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0` / 232 | NONE / 1 |
| business-races / T1 | `dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750` | copy `f48755f1-976b-4f47-98da-70ead936a4c3`<br>DV `699b8292-7fdb-4c22-af04-41f2eba761d4` | `b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524` / 129328 | NONE / 1 |
| business-races / T2 | `dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314` | copy `e09fd433-54cf-4e82-b8c4-098472505fa2`<br>DV `70d9185e-5ee7-4d79-9853-64e8e8db3630` | `a4ddf289321e3f5ede8528cb6268bd37d4165a727688477275a6497107fdaa4c` / 49 | NONE / 1 |
| late-hold / H2 | `dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc` | copy `c05307db-cb81-4220-b42e-be23f85821e3`<br>DV `18721e15-7b01-47d3-a727-8b836d73dbcb` | `b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838` / 44 | NONE / 1 |
| mixed / M1 | `dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d` | copy `5e712426-2109-4f0b-8791-df0a2be3a9f7`<br>DV `44727cb2-5f3b-4b48-b891-931583488ea9` | `e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725` / 40 | NONE / 1 |
| mixed / M1 | `dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6` | copy `ad08947a-4c4d-4b0f-8c7e-2d89f287819f`<br>DV `44727cb2-5f3b-4b48-b891-931583488ea9` | `e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725` / 40 | NONE / 1 |
| business-races / SURVIVOR | `dpp-lifecycle307-online/fixture/control-hold.csv@47026891-d10a-4400-a3cb-76915e3d51bf` | copy `91d954ee-cce5-4ceb-840d-fd6bdab778be`<br>DV `70b12ac5-1d86-462c-9176-fd5a681472cd` | `ad335d6334c5e1213b18c16dd8fe5bbd5bb1c1f53351ab8f13ce6e3b04120a90` / 54 | LEGAL_HOLD=ON / 0 |
| business-races / SURVIVOR | `dpp-lifecycle307-quarantine/fixture/control-retention.csv@2606d71f-207b-42a7-b671-4b7f4ca4d194` | copy `aa8ecd43-1c17-4d0b-b462-42600a426b1b`<br>DV `6837388a-80b1-4512-8976-ce2a7bae4e85` | `51c1c5e1a03e2347a4798ab8d9a312faef4853d028b7360ea84dfa8ea1616183` / 55 | GOVERNANCE until 2026-10-23T06:25:44Z / 0 |
| late-hold / SURVIVOR | `dpp-lifecycle307-online/fixture/h0.csv@369808db-a39a-4327-b9ed-5eb83aef7477` | copy `b3c4e113-f5c5-4421-a9d6-08403b1b9d0a`<br>DV `8d59d129-662b-4dae-a389-894943c716df` | `e35bd4635f1d7ab38c85a290a86a94275085b49bdae8e1a952712d5304816d8e` / 44 | NONE / 0 |
| late-hold / SURVIVOR | `dpp-lifecycle307-online/fixture/h1-a.csv@28d9f84a-1889-498d-b033-df26cbf209f2` | copy `0d8b77a8-195d-4a83-bd52-04f800d7dc68`<br>DV `6d7d6978-cbbf-4c78-b9d1-82fad7271f0a` | `838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0` / 44 | NONE / 0 |
| late-hold / SURVIVOR | `dpp-lifecycle307-quarantine/fixture/h1-b.csv@bae40a85-7539-4fe1-86b7-2cbb17df7812` | copy `e735d90a-8352-4a01-ad16-8573297bc684`<br>DV `6d7d6978-cbbf-4c78-b9d1-82fad7271f0a` | `838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0` / 44 | NONE / 0 |
| late-hold / SURVIVOR | `dpp-lifecycle307-quarantine/fixture/h2-b.csv@448112b2-baf3-4569-b06f-59011492cf80` | copy `a37c347d-dbff-492e-9c7a-317a1f188184`<br>DV `18721e15-7b01-47d3-a727-8b836d73dbcb` | `b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838` / 44 | NONE / 0 |
| late-hold / SURVIVOR | `dpp-lifecycle307-online/fixture/control-hold.csv@ca3b808e-734b-4d5e-8f2d-c78a16056ed6` | copy `a01751a1-9311-4357-8fb2-c833de6b8fc4`<br>DV `e6b6854f-b54a-462f-875d-9f74542b1fc3` | `961b213e4d82865370d8da052e57af716297ddff615298ed8c43e938f39b1b7d` / 49 | LEGAL_HOLD=ON / 0 |
| mixed / SURVIVOR | `dpp-lifecycle307-quarantine/fixture/m1-c.csv@81a8402e-390f-4247-9d34-552df5eadeb3` | copy `13fd8ecf-07fd-4981-b553-5ce66b67444f`<br>DV `44727cb2-5f3b-4b48-b891-931583488ea9` | `e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725` / 40 | GOVERNANCE until 2026-10-23T06:33:33Z / 0 |
| mixed / SURVIVOR | `dpp-lifecycle307-online/fixture/control-hold.csv@a59df17d-70d7-40c5-88e8-8cfe77a915dd` | copy `f3167994-3790-4a6c-918e-ea6d95248b79`<br>DV `57e8c123-6c69-44bc-98ba-d5db8c2b7aac` | `367afd5e5ed47c5f2cadab63af56e0a5e82a1a32afa88f0c3f99448aab883821` / 45 | LEGAL_HOLD=ON / 0 |

## 4. 确认前的有限验收顺序

1. 完成真实 Asynq/NATIVE queued/start/retry/recovery 和 authenticated DirectData 的非删除业务准备，创建合法 current rights/certification/caller/owner facts；不使用 PG fake dependencies 冒充原生业务路径。
2. 固定 reader/producer 权限、可信 owner 停止证明、reference release 顺序、H2 wire witness/pause、M1 lookup 前 mixed checkpoint；不承诺取消已在途 DELETE。
3. 实现新 case runner / restricted sender；离线验 exact version / max once / redirect / SDK retry=1 / UNKNOWN no-resend / current authorization fence，运行非删除和禁用 sender 回归。
4. 独立评审完整可执行候选；刷新新 source/binary/Core/native backup/IAM/gateway/execution、survivors、目标 digest、绝对 UTC 起止（≤40分钟），所有绑定写入新的精确确认文本。本轮窗口 `null`，没有滚动延长或默认同意。
5. 新确认前重读全库存/保护、Core reference/pins/owner/approval/attempt/cost/未知结果事实、native身份禁用状态、容器/网络/卷/无host ports；任何 drift/过期/未知均阻断，不自动刷新授权。
6. 只有另一次明确授权才允许启动受限 sender。按 P1/P2/T1/T2/H0/H1/H2/M1 既定顺序取证；任何未获准 target 0 DELETE。UNKNOWN 只读恢复，不重发。关闭 sender/权限/窗口、gateway deny-all，记录每份结果/历史/成本/保护/留存与 stop/PID0 回执。

本轮不提供可发送的 `CONFIRM_DELETE_*` 文本，因为现有 binary 无法执行这些新 case。这里列出了真实绑定与明确缺口，不能用用户对“准备工作”的确认替代真实删除授权。

## 5. 当前验收与原始失败

- 新 fixture / 16版本 / 8计划 / 原生 GET-LIST-protection / 三套 Core 与 native 恢复：**PASS_PREPARATION_ONLY**。
- 新工具 unit race：6 个顶层、含子项10个 PASS，0 FAIL/0 SKIP；`go vet` PASS。原 PG 9组17 PASS 及14份未变运行时源码的离线复核仍 PASS，未冒称本轮重跑真实 PostgreSQL并发。
- native P1/P2/T1/T2/H0/H1/H2/M1 真实竞争、可信停止/解 pin、原生 DELETE、完整 D1：**NOT_RUN / NOT_ACCEPTED**。
- 五份原始失败保留：disabled user attach404；pgcrypto恢复owner不一致；BusyBox tar无`-d`；启动日志stderr漏读；OSS policy set序列化字节比较。分别修复操作顺序、恢复role、SHA256比较、双stream采集、OSS `Policy.Equals`。前两套失败环境 excluded/stopped，12份版本保留；808a的有限 continuation 不重放 provision/restore，只做有新journal的只读核验。
- 失败 5f6a 的 inspector 在失败时尚未 disable；其 Core authority 已关闭、原生服务/全部helpers stopped/PID0/noports，严格排除当前候选，不能声称该失败环境 native IAM 已全部 disabled，也不得将它重新启动为可执行目标。
- 没有新的独立评审结论：继承的是有限设计 bca29f4d 的独立通过，新工具/绑定须另行复评。PR当前 HEAD CI 在 PR checkpoint 按精确 HEAD 读回，不拿冻结源码 de5f 或 #318 main CI 替代。

离线执行：`python -X utf8 docs/acceptance/evidence/issue307-d1/remaining-native01/audit.py`。原始证据及逐文件 hash 见 [evidence-index.json](evidence/issue307-d1/remaining-native01/evidence-index.json)。
