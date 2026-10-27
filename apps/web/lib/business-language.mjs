// Presentation only. Never use these labels as command values, keys or identities.
const terms={
 RAW:'原始数据',STANDARDIZED:'标准数据',CURATED:'加工成果',PRODUCT:'数据产品',
 READY:'可用',ACTIVE:'有效',DRAFT:'草稿',CREATED:'已创建',PROCESSING:'处理中',SUBMITTING:'提交中',RUNNING:'运行中',SUCCEEDED:'成功',FAILED:'失败',FAIL:'未通过',PASS:'通过',PASS_WITH_WARNING:'通过，有提醒',PENDING:'待处理',QUEUED:'待运行',UNKNOWN:'无法确认',UNRESOLVED:'待核实',
 CERTIFIED:'认证通过',NOT_CERTIFIED:'认证未通过',PUBLISHED:'已发布',VALIDATING:'检查中',INVALID:'已作废',INVALIDATED:'已作废',REVOKED:'已撤销',SUPERSEDED:'已被替代',APPROVED:'已批准',REJECTED:'已拒绝',BLOCKED:'已阻断',ALLOWED:'允许',DENIED:'拒绝',CONFLICT:'存在冲突',CANCELLED:'已取消',COMPLETED:'已完成',
 ISSUED:'授权已释放',PREPARED:'已准备',ISSUANCE_PENDING:'签发待核实',CONTAINMENT_PENDING:'访问能力待收回',REQUESTED:'意图已保存',NOT_DISPATCHED:'未执行',CONFIRMED:'已确认',AUTO_CONFIRMED:'已自动确认',REVIEW:'待审核',WAITING_REVIEW:'待审核',HEALTHY:'正常',UNHEALTHY:'异常',ACTION_REQUIRED:'待处理',PAUSED:'已暂停',STOPPED:'已停止',
 DIAGNOSIS:'质量诊断',REPAIR:'修复方案',DETECT:'质量诊断',ASSIGN_SELF:'受理修复',EXECUTE_REPAIR:'修复处理',RECHECK:'质量复检',
 INFO:'一般',WARN:'提醒',ERROR:'错误',DEBUG:'调试',TRACE:'追踪',READ_ONLY:'只读','NOT EVALUATED':'尚未核验',OUTCOME_UNKNOWN:'结果待核实',UNAVAILABLE:'暂不可用',INTERRUPTED:'已中断',CLEAR:'暂无待办',PRIMARY:'主成果',OUTPUT:'输出成果',INPUT:'输入成果',ALL_RESOURCE:'全部资源',FINALIZED:'已冻结',
 company:'企业信息',tax:'税务记录',energy:'能耗记录',resolved:'实体匹配结果',DRIFT:'结构有变化',MATCH:'结构一致',CRITICAL:'严重',HIGH:'高',MEDIUM:'中',LOW:'低'
};
const referenceKinds={ACC:'验收单',VER:'成果版本',DEL:'交付记录',RCT:'签收记录',SUB:'验收提交',DEC:'验收决定',ARC:'验收档案',CRIT:'验收标准'};
// React children can contain elements as well as text. Preserve elements and
// source data; only strings in presentation slots receive business labels.
export function displayText(value){
 if(typeof value==='string')return businessText(value);
 if(Array.isArray(value))return value.map(displayText);
 return value;
}
export function referenceLabel(value){
 if(typeof value!=='string')return value==null?'':String(value);
 const receipt=/^DEMO-([A-Z]+)-(\d+)$/.exec(value);if(receipt&&referenceKinds[receipt[1]])return `${referenceKinds[receipt[1]]} ${receipt[2]}`;
 const local=/^(MATCH|TRACE|SOURCE|DEC)-DEMO-(\d+)$/.exec(value);if(local)return `${({MATCH:'匹配记录',TRACE:'日志记录',SOURCE:'来源记录',DEC:'匹配决定'})[local[1]]} ${local[2]}`;
 return value;
}
export function businessLabel(value){
 if(typeof value!=='string')return value==null?'':String(value);
 return terms[value]??referenceLabel(value);
}
export function businessText(value){
 if(typeof value!=='string')return value==null?'':String(value);
 // Source paths and identifiers are evidence, not translatable interface copy.
 if(/^[a-z0-9_.-]+\/[a-z0-9_./-]+/.test(value)||/^[a-z]+:\/\//.test(value))return value;
 // A code may contain a word that is also a known status. Never translate a
 // fragment of a material/reference identity (for example STORAGE-UNAVAILABLE).
 if(/^[A-Za-z][A-Za-z0-9_]*(?:[-:/][A-Za-z0-9_.-]+)+$/.test(value)&&!['source-reference','negative-energy'].includes(value))return referenceLabel(value);
 value=value.replaceAll('Gold output 成果版本','输出成果版本').replaceAll('发布批次号tes','发布说明').replaceAll('发布记录 Notes','发布说明').replaceAll('发布记录 Draft','发布草稿').replaceAll('发布记录 No','发布批次号');
 let text=businessLabel(value).replace(/\b(?:DEMO-[A-Z]+-\d+|(?:MATCH|TRACE|SOURCE|DEC)-DEMO-\d+)\b/g,referenceLabel)
  .replace(/(?<![A-Za-z0-9_./:-])[A-Z][A-Z_]+(?![A-Za-z0-9_./:-])/g,t=>terms[t]??t)
  .replaceAll('演示管理员','空间管理员（示例）').replaceAll('演示用户','当前用户（示例）')
  .replaceAll('园区能耗质量演示数据集','园区能耗质量数据集（示例）');
 text=text.replaceAll('浏览器测试模板','质量整改方案（示例）').replaceAll('source-reference','来源引用检查').replaceAll('Gold Dataset','成果认证标准（示例）').replaceAll('negative-energy','能耗非负检查').replaceAll('浏览器验收产品','企业数据产品（示例）');
 const copy={
  'Target Period':'目标期间','Engine Type':'处理方式','Engine Execution':'外部运行编号','Gold Production Chain':'成果生产依据','Input Certification':'输入认证','Annotation Contribution':'标注贡献','Annotation Campaign':'标注任务','Input DatasetVersion':'输入成果版本','Purpose / Action':'用途与动作','Tasks / Results / Reviews / Outputs':'任务、结果、审核与产出','FINALIZED Snapshot':'冻结快照','Gold Build Execution':'成果构建记录','Annotation / Human Review':'标注与人工审核','Schema':'结构规则','Taxonomy':'分类规则','Validation':'核验','worker':'处理服务',
  'Quality Check':'质量评测','Compliance Check':'合规检查','Quality Rule Set':'质量规则集','Quality Engine':'评测引擎','Compliance Policy':'合规策略','Gold output Role':'输出成果用途','Gold output DatasetVersion':'输出成果版本','Release No':'发布批次号','Release Notes':'发布说明','Release Draft':'发布草稿','Direct Data':'数据下载','EXACT ASSESSMENT':'指定评测','CONTEXT':'关联信息','workbench':'业务工作台','Workbench':'业务工作台','Assessment':'评测记录','Result':'结果记录',
  '保存演示资料':'保存资料','接收演示站内提醒':'接收站内提醒','演示工作空间':'当前工作空间','管理演示资料与工作空间展示信息。':'管理个人资料与工作空间信息。','重置演示':'重置预览数据','查看业务验收演示':'查看业务验收',
  '保存演示方案':'保存方案','保存演示配置':'保存配置','新增演示用户':'新增用户','演示成员':'成员','演示范围':'适用范围','维护演示成员、所属机构与角色。':'维护成员、所属机构与角色。','定位演示运行问题':'定位运行问题',
  '演示办理身份':'办理身份（预览）','演示办理人':'办理人（预览）','演示业务需求方':'业务需求方（示例）','固定演示成果':'对应成果','演示交付':'对应交付','演示成果版本':'成果版本','演示输出':'输出成果','演示步骤':'步骤','演示记录已更新':'当前记录已更新',
  '填写演示核对记录或材料编号，不上传文件':'填写核对记录或材料编号','模拟发布当前版本':'预演发布当前版本','模拟运行独立评测':'预演质量评测','运行模拟评测':'预演质量评测','模拟核对登记':'核对登记','创建演示任务':'创建任务','进入已保存的演示任务':'查看已创建的任务','模拟运行':'预演运行','模拟检查':'预演检查','模拟服务失败':'预演失败场景','模拟汇聚观察完成':'汇聚观察已完成（预演）','模拟来源连接超时':'来源连接超时（预演）','模拟容量阈值提醒':'容量阈值提醒（预演）','模拟备份检查通过':'备份检查通过（预演）','生成模拟预览':'生成结果预览','运行模拟探查':'预演数据探查','模拟执行观察':'预演监控检查','调用演示':'接口预演','启用演示任务通知':'启用任务通知',
  '纯前端模拟':'预览模式','合成台账':'资源台账','演示对象存储':'成果文件存储','演示业务数据库':'业务数据库','合成用量':'示例用量','按合成版本评测':'按所选成果版本评测','按实际模拟尝试归集用量，不是真实账单':'按预演活动汇总用量，金额待核定','任务产出成为可追溯的演示版本':'追踪任务产出及成果版本','逐项登记演示依据，交付时重新检查当前结论':'登记使用依据，交付时核对当前资格','记录演示依据':'登记使用依据','方案来源表的合成样例':'方案中已选定的来源表','本地合成样例运行完成':'本次预演已完成','同一演示流水线':'同一业务流程',
  '从质量上下文复制明确版本 UUID':'复制成果详情中的完整版本编号','输入明确的 UUID':'填写完整记录编号','交付操作 UUID':'交付记录编号','成果版本 UUID':'成果版本编号','快照 UUID':'档案编号','合约版本 UUID':'合约版本编号','UUID':'编号','SHA256':'校验摘要','幂等键':'提交凭据','未派发':'未执行','派发':'启动执行',
  'Data Resource':'数据资源','Raw Dataset':'原始数据','Data Contract':'数据合约','Data Product':'数据产品','ProductRelease':'发布记录','ProductVersion':'产品版本','DatasetVersion':'成果版本','WorkflowVersion':'工作流版本','QualityAssessment':'质量评测','EvidenceSnapshot':'证据快照','RightsSnapshot':'权利快照','DataResource':'数据资源','Workflow Execution':'工作流运行','Execution':'运行记录','Readiness':'发布检查','Release':'发布记录','Dataset':'数据集','Workflow':'工作流','ContractVersion':'合约版本','Attempt':'尝试记录','attempt':'尝试记录','Blockers':'阻断原因','blocker':'阻断原因','Blocker':'阻断原因','Gate':'检查项','Evidence':'证据记录','schema':'数据结构','SOURCE FACT':'来源记录','BUSINESS BASIS':'业务依据','QUALITY REPORT':'评测报告','QUALITY HISTORY':'评测历史','QUALITY CONTEXT':'质量上下文','READ ONLY':'只读','WORKBENCH':'业务工作台','Workspace':'工作空间','workspace':'工作空间',
  '后端接入后统一校验':'接入后统一核验','待后端与运维方案确定后接入。':'备份与恢复服务尚未配置。','不具备服务端备份、日志归档或灾难恢复能力。':'当前不提供集中备份、日志归档或灾难恢复。','偏移':'起始位置'
 };
 for(const [from,to] of Object.entries(copy).sort((a,b)=>b[0].length-a[0].length))text=/^[A-Za-z]+$/.test(from)?text.replace(new RegExp('(?<![A-Za-z0-9_./:-])'+from+'(?![A-Za-z0-9_./:-])','g'),to):text.replaceAll(from,to);
 return text.replaceAll('compliance评测记录Id','合规检查编号').replaceAll('compliance尝试记录Id','合规检查编号').replaceAll('quality评测记录Id','质量评测编号').replaceAll('quality尝试记录Id','质量评测编号').replaceAll('发布 发布记录','确认发布').replaceAll('治理与成果交付演示','治理与成果交付').replaceAll('无法确认 · 无法确认','无法确认').replaceAll('已发布 发布记录','已发布批次').replaceAll('源端 数据结构','源表结构').replaceAll('无观察 评测记录','尚无结果的扫描尝试').replaceAll('无观察 尝试记录','尚无结果的扫描尝试').replaceAll('无观察请求','尚无结果的扫描请求').replaceAll('来源版本 ID','来源版本编号').replaceAll('尝试 ID','评测记录编号').replaceAll('合成样例','示例数据').replaceAll('静态计划样例','计划预览');
}
