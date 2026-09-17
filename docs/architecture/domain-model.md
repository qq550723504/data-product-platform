# 核心领域模型 V1.0

## 1. 主业务链

```text
Workspace
  ↓
Project
  ↓
UseCase
  ↓
ProductOpportunity
  │
  ├── Rights
  │
  └── DataResource
          ↓
        Dataset
          ↓
    DatasetVersion
          ↓
   Entity / Workflow
          ↓
      Execution
          ↓
     DataContract
          ↓
     DataProduct
          ↓
   ProductVersion
          ↓
   ProductRelease
```

横向能力：`Quality · Compliance · Cost · Evidence · Audit`。

## 2. 重要区分

### DataResource

回答"有什么业务数据资源"。不等于 Table。

### Dataset

回答"平台中的逻辑数据集是什么"。

### DatasetVersion

回答"某一次生产真实产生的是哪批不可变数据"。

### DataProduct

稳定的产品身份。

### ProductVersion

稳定的产品规格版本，绑定 Contract / Workflow / Indicator / Policy 等产品定义。

### ProductRelease

某次实际发布快照，绑定具体 DatasetVersion、Rights、Quality、Compliance、Evidence。

## 3. 实体模型

```text
EntityType
   ↓
Canonical Entity
   ↓
EntityMapping
```

Core 不写死 COMPANY。

Park Industry Pack 可提供：

- COMPANY
- PARK
- BUILDING
- METER
- EQUIPMENT

EntityMapping 必须记录：

- source reference
- source key
- match method
- confidence
- policy version
- review decision
- evidence

## 4. 权利模型

```text
Authorization
├── Resource
├── Purpose
├── Action
├── Scope
├── Consumer
├── Validity
└── Evidence
```

Rights 判断的语义不是"谁拥有数据"，而是：

```text
Subject + Resource + Purpose + Action + Context → Decision
```

## 5. 工作流模型

```text
Workflow
  ↓
WorkflowVersion
  ↓
TaskDefinition
  ↓
WorkflowRun
  ↓
TaskRun / Execution
```

Task 类型可包括：

- DATA_INPUT / DATA_OUTPUT
- TRANSFORM / SQL / PYTHON
- ENTITY_RESOLUTION
- INDICATOR
- RIGHTS_CHECK
- QUALITY_CHECK
- COMPLIANCE_CHECK
- HUMAN_REVIEW
- APPROVAL
- EXTERNAL

业务 Workflow 不等于 Apache Hop Workflow。

## 6. 产品模型

```text
DataProduct
  ↓
ProductVersion
  ├── ProductAsset(DATASET/API/REPORT/...)
  ↓
ProductRelease
```

Product Release 必须经过统一 ReleaseReadiness。

## 7. 证据模型

```text
Claim / Business Object
          ↓
      Evidence
          ↓
 EvidenceRelation
```

Evidence 用于证明事实；AuditEvent 用于记录"谁做了什么"。两者不能混用。

## 8. 版本原则

下列对象使用独立版本，不使用 `updated_at` 代替版本管理：

- DatasetVersion
- WorkflowVersion
- ContractVersion
- ProductVersion
- PolicyVersion（逐步实现）

Release 形成 EvidenceSnapshot，冻结发布时的事实。
