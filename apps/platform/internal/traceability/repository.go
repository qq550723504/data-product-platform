package traceability

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
)

type Repository struct {
	pool         *pgxpool.Pool
	evidenceRepo *evidence.QueryRepository
	costRepo     *cost.QueryRepository
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:         pool,
		evidenceRepo: evidence.NewQueryRepository(pool),
		costRepo:     cost.NewQueryRepository(pool),
	}
}

type ReleaseTrace struct {
	ReleaseID        uuid.UUID              `json:"releaseId"`
	ReleaseNo        string                 `json:"releaseNo"`
	Status           string                 `json:"status"`
	ProductID        uuid.UUID              `json:"productId"`
	ProductVersionID uuid.UUID              `json:"productVersionId"`
	EvidenceSnapshot *EvidenceSnapshotTrace `json:"evidenceSnapshot,omitempty"`
	DatasetVersions  []DatasetVersionTrace  `json:"datasetVersions"`
	Executions       []ExecutionTrace       `json:"executions"`
	EntityMatchJobs  []EntityMatchJobTrace  `json:"entityMatchJobs"`
	EntityMappings   []EntityMappingTrace   `json:"entityMappings"`
	Evidence         []evidence.Item        `json:"evidence"`
	CostEvents       []cost.Item            `json:"costEvents"`
	AuditEvents      []AuditEventTrace      `json:"auditEvents"`
}

type EvidenceSnapshotTrace struct {
	ID             uuid.UUID               `json:"id"`
	RootHash       string                  `json:"rootHash"`
	IntegrityValid bool                    `json:"integrityValid"`
	Manifest       map[string]any          `json:"manifest"`
	Items          []evidence.SnapshotItem `json:"items"`
	CreatedAt      time.Time               `json:"createdAt"`
}

type DatasetVersionTrace struct {
	ID                     uuid.UUID  `json:"id"`
	DatasetID              uuid.UUID  `json:"datasetId"`
	DatasetCode            string     `json:"datasetCode"`
	DatasetType            string     `json:"datasetType"`
	SourceResourceID       *uuid.UUID `json:"sourceResourceId,omitempty"`
	VersionNo              int64      `json:"versionNo"`
	Status                 string     `json:"status"`
	ReleaseRole            string     `json:"releaseRole,omitempty"`
	StorageURI             string     `json:"storageUri,omitempty"`
	ChecksumAlgorithm      string     `json:"checksumAlgorithm,omitempty"`
	ChecksumValue          string     `json:"checksumValue,omitempty"`
	GeneratedByExecutionID *uuid.UUID `json:"generatedByExecutionId,omitempty"`
}

type ExecutionTrace struct {
	ID                          uuid.UUID                    `json:"id"`
	WorkflowVersionID           uuid.UUID                    `json:"workflowVersionId"`
	WorkflowVersion             string                       `json:"workflowVersion"`
	WorkflowDefinitionHash      string                       `json:"workflowDefinitionHash"`
	Status                      string                       `json:"status"`
	Attempt                     int                          `json:"attempt"`
	EngineType                  string                       `json:"engineType"`
	EngineExecutionID           string                       `json:"engineExecutionId,omitempty"`
	TargetPeriod                string                       `json:"targetPeriod"`
	OutputDatasetVersionID      *uuid.UUID                   `json:"outputDatasetVersionId,omitempty"`
	Metrics                     map[string]any               `json:"metrics"`
	DependencyPreparationStatus string                       `json:"dependencyPreparationStatus"`
	DependencyBindings          []ExecutionDependencyTrace   `json:"dependencyBindings"`
	MappingUsages               []ExecutionMappingUsageTrace `json:"mappingUsages"`
}

type ExecutionDependencyTrace struct {
	Name             string     `json:"name"`
	DatasetVersionID *uuid.UUID `json:"datasetVersionId,omitempty"`
	Reference        string     `json:"reference"`
	Version          string     `json:"version"`
	ContentSHA256    string     `json:"contentSha256"`
}

type ExecutionMappingUsageTrace struct {
	ID                         uuid.UUID `json:"id"`
	InputName                  string    `json:"inputName"`
	InputDatasetVersionID      uuid.UUID `json:"inputDatasetVersionId"`
	ResolutionDatasetVersionID uuid.UUID `json:"resolutionDatasetVersionId"`
	DecisionID                 uuid.UUID `json:"decisionId"`
	EntityID                   uuid.UUID `json:"entityId"`
	SourceType                 string    `json:"sourceType"`
	SourceRef                  string    `json:"sourceRef"`
	SourceKey                  string    `json:"sourceKey"`
}

type EntityMatchJobTrace struct {
	ID                     uuid.UUID  `json:"id"`
	WorkspaceID            uuid.UUID  `json:"workspaceId"`
	EntityTypeID           uuid.UUID  `json:"entityTypeId"`
	InputDatasetVersionID  uuid.UUID  `json:"inputDatasetVersionId"`
	OutputDatasetVersionID *uuid.UUID `json:"outputDatasetVersionId,omitempty"`
	SourceType             string     `json:"sourceType"`
	SourceRef              string     `json:"sourceRef"`
	PolicyRef              string     `json:"policyRef"`
	PolicyVersion          string     `json:"policyVersion"`
	Status                 string     `json:"status"`
}

// EntityMappingTrace reports the immutable mapping decision that a release-time
// match job actually applied. It deliberately contains DecisionID: the
// entity_mapping projection is mutable, so a release must show the decision it
// was produced with, not whatever mapping happens to be current now.
type EntityMappingTrace struct {
	ID                 uuid.UUID  `json:"id"`
	DecisionID         uuid.UUID  `json:"decisionId"`
	EntityID           uuid.UUID  `json:"entityId"`
	SourceType         string     `json:"sourceType"`
	SourceRef          string     `json:"sourceRef"`
	SourceKey          string     `json:"sourceKey"`
	SourceName         string     `json:"sourceName,omitempty"`
	MatchMethod        string     `json:"matchMethod"`
	MatchRuleID        string     `json:"matchRuleId,omitempty"`
	MatchPolicyVersion string     `json:"matchPolicyVersion"`
	Confidence         float64    `json:"confidence"`
	Status             string     `json:"status"`
	ReviewedBy         *uuid.UUID `json:"reviewedBy,omitempty"`
	ReviewedAt         *time.Time `json:"reviewedAt,omitempty"`
	ReviewerReason     string     `json:"reviewerReason,omitempty"`
	EvidenceID         *uuid.UUID `json:"evidenceId,omitempty"`
	SourceOrigin       string     `json:"sourceOrigin"`
	SourceJobID        *uuid.UUID `json:"sourceJobId,omitempty"`
	DecidedAt          time.Time  `json:"decidedAt"`
}

type AuditEventTrace struct {
	ID         uuid.UUID      `json:"id"`
	Action     string         `json:"action"`
	ObjectType string         `json:"objectType"`
	ObjectID   uuid.UUID      `json:"objectId"`
	ActorType  string         `json:"actorType"`
	ActorID    *uuid.UUID     `json:"actorId,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Metadata   map[string]any `json:"metadata"`
	OccurredAt time.Time      `json:"occurredAt"`
}

func (r *Repository) ProductRelease(ctx context.Context, releaseID uuid.UUID) (ReleaseTrace, error) {
	var trace ReleaseTrace
	var snapshotID *uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id, release_no, status, product_id, product_version_id, evidence_snapshot_id
		FROM product_release
		WHERE id=$1
	`, releaseID).Scan(&trace.ReleaseID, &trace.ReleaseNo, &trace.Status, &trace.ProductID, &trace.ProductVersionID, &snapshotID)
	if err != nil {
		return ReleaseTrace{}, fmt.Errorf("query ProductRelease trace root: %w", err)
	}

	if snapshotID != nil {
		snapshot, err := r.evidenceRepo.GetSnapshot(ctx, *snapshotID)
		if err != nil {
			return ReleaseTrace{}, err
		}
		trace.EvidenceSnapshot = &EvidenceSnapshotTrace{
			ID:             snapshot.ID,
			RootHash:       snapshot.RootHash,
			IntegrityValid: snapshot.IntegrityValid,
			Manifest:       snapshot.Manifest,
			Items:          snapshot.Items,
			CreatedAt:      snapshot.CreatedAt,
		}
	}

	trace.DatasetVersions, err = r.releaseDatasetLineage(ctx, releaseID)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.Executions, err = r.executionsForDatasets(ctx, trace.DatasetVersions)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.EntityMatchJobs, err = r.entityMatchJobsForDatasets(ctx, trace.DatasetVersions)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.EntityMappings, err = r.entityMappingsForJobs(ctx, trace.EntityMatchJobs)
	if err != nil {
		return ReleaseTrace{}, err
	}
	usageMappings, err := r.entityMappingsForExecutionUsages(ctx, trace.Executions)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.EntityMappings = appendUniqueEntityMappings(trace.EntityMappings, usageMappings)

	trace.Evidence, err = r.collectEvidence(ctx, trace)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.CostEvents, err = r.collectCosts(ctx, trace.Executions)
	if err != nil {
		return ReleaseTrace{}, err
	}
	trace.AuditEvents, err = r.collectAudit(ctx, trace)
	if err != nil {
		return ReleaseTrace{}, err
	}
	return trace, nil
}

func (r *Repository) releaseDatasetLineage(ctx context.Context, releaseID uuid.UUID) ([]DatasetVersionTrace, error) {
	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE lineage(version_id) AS (
			SELECT dataset_version_id
			FROM product_release_dataset
			WHERE release_id=$1
			UNION
			SELECT dvl.input_version_id
			FROM dataset_version_lineage dvl
			JOIN lineage l ON l.version_id=dvl.output_version_id
		)
		SELECT dv.id, dv.dataset_id, d.code, d.dataset_type, d.source_resource_id,
		       dv.version_no, dv.status,
		       COALESCE((
		           SELECT string_agg(prd.role, ',' ORDER BY prd.role)
		           FROM product_release_dataset prd
		           WHERE prd.release_id=$1 AND prd.dataset_version_id=dv.id
		       ), ''),
		       COALESCE(dv.storage_uri,''), COALESCE(dv.checksum_algorithm,''), COALESCE(dv.checksum_value,''),
		       dv.generated_by_execution_id
		FROM lineage l
		JOIN dataset_version dv ON dv.id=l.version_id
		JOIN dataset d ON d.id=dv.dataset_id
		ORDER BY d.dataset_type, d.code, dv.version_no, dv.id
	`, releaseID)
	if err != nil {
		return nil, fmt.Errorf("query ProductRelease DatasetVersion lineage: %w", err)
	}
	defer rows.Close()
	result := make([]DatasetVersionTrace, 0)
	for rows.Next() {
		var item DatasetVersionTrace
		if err := rows.Scan(
			&item.ID, &item.DatasetID, &item.DatasetCode, &item.DatasetType, &item.SourceResourceID,
			&item.VersionNo, &item.Status, &item.ReleaseRole, &item.StorageURI,
			&item.ChecksumAlgorithm, &item.ChecksumValue, &item.GeneratedByExecutionID,
		); err != nil {
			return nil, fmt.Errorf("scan ProductRelease DatasetVersion lineage: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ProductRelease DatasetVersion lineage: %w", err)
	}
	return result, nil
}

func (r *Repository) executionsForDatasets(ctx context.Context, datasets []DatasetVersionTrace) ([]ExecutionTrace, error) {
	ids := make([]uuid.UUID, 0)
	seen := map[uuid.UUID]struct{}{}
	for _, dataset := range datasets {
		if dataset.GeneratedByExecutionID == nil {
			continue
		}
		if _, ok := seen[*dataset.GeneratedByExecutionID]; ok {
			continue
		}
		seen[*dataset.GeneratedByExecutionID] = struct{}{}
		ids = append(ids, *dataset.GeneratedByExecutionID)
	}
	if len(ids) == 0 {
		return []ExecutionTrace{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.workflow_version_id, wv.version, wv.definition_sha256,
		       e.status, e.attempt, e.engine_type, COALESCE(e.engine_execution_id,''),
		       e.target_period, e.output_dataset_version_id, e.metrics
		FROM execution e
		JOIN workflow_version wv ON wv.id=e.workflow_version_id
		WHERE e.id=ANY($1::uuid[])
		ORDER BY e.created_at, e.id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query release executions: %w", err)
	}
	result := make([]ExecutionTrace, 0)
	for rows.Next() {
		var item ExecutionTrace
		var metrics []byte
		if err := rows.Scan(
			&item.ID, &item.WorkflowVersionID, &item.WorkflowVersion, &item.WorkflowDefinitionHash,
			&item.Status, &item.Attempt, &item.EngineType, &item.EngineExecutionID,
			&item.TargetPeriod, &item.OutputDatasetVersionID, &metrics,
		); err != nil {
			return nil, fmt.Errorf("scan release execution: %w", err)
		}
		if len(metrics) > 0 {
			if err := json.Unmarshal(metrics, &item.Metrics); err != nil {
				return nil, fmt.Errorf("decode release execution metrics: %w", err)
			}
		}
		if item.Metrics == nil {
			item.Metrics = map[string]any{}
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range result {
		result[i].DependencyPreparationStatus, result[i].DependencyBindings, result[i].MappingUsages, err = r.executionDependencyTrace(ctx, result[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) executionDependencyTrace(ctx context.Context, executionID uuid.UUID) (string, []ExecutionDependencyTrace, []ExecutionMappingUsageTrace, error) {
	status := "NOT_AVAILABLE"
	var prepared bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM execution_dependency_preparation WHERE execution_id=$1)
	`, executionID).Scan(&prepared); err != nil {
		return "", nil, nil, fmt.Errorf("query execution dependency preparation: %w", err)
	}
	if prepared {
		status = "PREPARED"
	}
	dependencies, err := r.pool.Query(ctx, `
		SELECT dependency_name, dataset_version_id, reference, version, content_sha256
		FROM execution_dependency_binding
		WHERE execution_id=$1
		ORDER BY dependency_name
	`, executionID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("query execution dependency bindings: %w", err)
	}
	defer dependencies.Close()
	dependencyResult := make([]ExecutionDependencyTrace, 0)
	for dependencies.Next() {
		var item ExecutionDependencyTrace
		if err := dependencies.Scan(&item.Name, &item.DatasetVersionID, &item.Reference, &item.Version, &item.ContentSHA256); err != nil {
			return "", nil, nil, fmt.Errorf("scan execution dependency binding: %w", err)
		}
		dependencyResult = append(dependencyResult, item)
	}
	if err := dependencies.Err(); err != nil {
		return "", nil, nil, fmt.Errorf("iterate execution dependency bindings: %w", err)
	}
	usages, err := r.pool.Query(ctx, `
		SELECT id, input_name, input_dataset_version_id, resolution_dataset_version_id,
		       decision_id, entity_id, source_type, source_ref, source_key
		FROM execution_mapping_usage
		WHERE execution_id=$1
		ORDER BY input_name, source_ref, source_key
	`, executionID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("query execution mapping usages: %w", err)
	}
	defer usages.Close()
	usageResult := make([]ExecutionMappingUsageTrace, 0)
	for usages.Next() {
		var item ExecutionMappingUsageTrace
		if err := usages.Scan(&item.ID, &item.InputName, &item.InputDatasetVersionID, &item.ResolutionDatasetVersionID,
			&item.DecisionID, &item.EntityID, &item.SourceType, &item.SourceRef, &item.SourceKey); err != nil {
			return "", nil, nil, fmt.Errorf("scan execution mapping usage: %w", err)
		}
		usageResult = append(usageResult, item)
	}
	if err := usages.Err(); err != nil {
		return "", nil, nil, fmt.Errorf("iterate execution mapping usages: %w", err)
	}
	return status, dependencyResult, usageResult, nil
}

func (r *Repository) entityMatchJobsForDatasets(ctx context.Context, datasets []DatasetVersionTrace) ([]EntityMatchJobTrace, error) {
	ids := make([]uuid.UUID, 0, len(datasets))
	for _, dataset := range datasets {
		ids = append(ids, dataset.ID)
	}
	if len(ids) == 0 {
		return []EntityMatchJobTrace{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_version_id,
		       source_type, source_ref, policy_ref, policy_version, status
		FROM entity_match_job
		WHERE output_dataset_version_id=ANY($1::uuid[])
		ORDER BY created_at, id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query release entity match jobs: %w", err)
	}
	defer rows.Close()
	result := make([]EntityMatchJobTrace, 0)
	for rows.Next() {
		var item EntityMatchJobTrace
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.EntityTypeID, &item.InputDatasetVersionID, &item.OutputDatasetVersionID,
			&item.SourceType, &item.SourceRef, &item.PolicyRef, &item.PolicyVersion, &item.Status,
		); err != nil {
			return nil, fmt.Errorf("scan release entity match job: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// entityMappingsForJobs reads the immutable decisions produced by the release's
// own match jobs. A release must never be reconstructed from the mutable
// entity_mapping projection: the current mapping can move to another entity long
// after the release was produced, while the decision that produced its
// DatasetVersion stays frozen. Decisions that cannot prove a source job
// (idempotency/legacy rows with source_job_id IS NULL) are deliberately not
// attributed to a release instead of being guessed from the current table.
func (r *Repository) entityMappingsForJobs(ctx context.Context, jobs []EntityMatchJobTrace) ([]EntityMappingTrace, error) {
	ids := make([]uuid.UUID, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	if len(ids) == 0 {
		return []EntityMappingTrace{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.mapping_id, d.entity_id, d.source_type, d.source_ref, d.source_key, COALESCE(d.source_name,''),
		       d.match_method, COALESCE(d.match_rule_id,''), d.match_policy_version, COALESCE(d.confidence,0),
		       d.status, d.reviewed_by, d.reviewed_at, COALESCE(d.reviewer_reason,''), d.evidence_id,
		       d.source_origin, d.source_job_id, d.decided_at
		FROM entity_mapping_decision d
		WHERE d.source_job_id=ANY($1::uuid[])
		ORDER BY d.source_key, d.decided_seq, d.id
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query release entity mapping decisions: %w", err)
	}
	defer rows.Close()
	result := make([]EntityMappingTrace, 0)
	for rows.Next() {
		var item EntityMappingTrace
		if err := rows.Scan(
			&item.DecisionID, &item.ID, &item.EntityID, &item.SourceType, &item.SourceRef, &item.SourceKey, &item.SourceName,
			&item.MatchMethod, &item.MatchRuleID, &item.MatchPolicyVersion, &item.Confidence,
			&item.Status, &item.ReviewedBy, &item.ReviewedAt, &item.ReviewerReason, &item.EvidenceID,
			&item.SourceOrigin, &item.SourceJobID, &item.DecidedAt,
		); err != nil {
			return nil, fmt.Errorf("scan release entity mapping decision: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release entity mapping decisions: %w", err)
	}
	return result, nil
}

func (r *Repository) entityMappingsForExecutionUsages(ctx context.Context, executions []ExecutionTrace) ([]EntityMappingTrace, error) {
	ids := make([]uuid.UUID, 0, len(executions))
	for _, execution := range executions {
		ids = append(ids, execution.ID)
	}
	if len(ids) == 0 {
		return []EntityMappingTrace{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.mapping_id, d.entity_id, d.source_type, d.source_ref, d.source_key, COALESCE(d.source_name,''),
		       d.match_method, COALESCE(d.match_rule_id,''), d.match_policy_version, COALESCE(d.confidence,0),
		       d.status, d.reviewed_by, d.reviewed_at, COALESCE(d.reviewer_reason,''), d.evidence_id,
		       d.source_origin, d.source_job_id, d.decided_at
		FROM execution_mapping_usage u
		JOIN entity_mapping_decision d
		  ON d.workspace_id=u.workspace_id AND d.id=u.decision_id
		WHERE u.execution_id=ANY($1::uuid[])
		ORDER BY u.execution_id, u.input_name, u.source_ref, u.source_key
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query execution mapping decisions: %w", err)
	}
	defer rows.Close()
	result := make([]EntityMappingTrace, 0)
	for rows.Next() {
		var item EntityMappingTrace
		if err := rows.Scan(
			&item.DecisionID, &item.ID, &item.EntityID, &item.SourceType, &item.SourceRef, &item.SourceKey, &item.SourceName,
			&item.MatchMethod, &item.MatchRuleID, &item.MatchPolicyVersion, &item.Confidence,
			&item.Status, &item.ReviewedBy, &item.ReviewedAt, &item.ReviewerReason, &item.EvidenceID,
			&item.SourceOrigin, &item.SourceJobID, &item.DecidedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution mapping decision: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate execution mapping decisions: %w", err)
	}
	return result, nil
}

func appendUniqueEntityMappings(existing, additions []EntityMappingTrace) []EntityMappingTrace {
	seen := make(map[uuid.UUID]struct{}, len(existing)+len(additions))
	for _, item := range existing {
		seen[item.DecisionID] = struct{}{}
	}
	result := append([]EntityMappingTrace(nil), existing...)
	for _, item := range additions {
		if _, ok := seen[item.DecisionID]; ok {
			continue
		}
		seen[item.DecisionID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (r *Repository) collectEvidence(ctx context.Context, trace ReleaseTrace) ([]evidence.Item, error) {
	result := make([]evidence.Item, 0)
	seen := map[uuid.UUID]struct{}{}
	add := func(items []evidence.Item) {
		for _, item := range items {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			result = append(result, item)
		}
	}

	if trace.EvidenceSnapshot != nil {
		for _, snapshotItem := range trace.EvidenceSnapshot.Items {
			item, err := r.evidenceRepo.Get(ctx, snapshotItem.EvidenceID)
			if err != nil {
				return nil, err
			}
			item.RelationType = "SNAPSHOT:" + snapshotItem.Category
			add([]evidence.Item{item})
		}
	}
	for _, dataset := range trace.DatasetVersions {
		items, err := r.evidenceRepo.ListForObject(ctx, "DATASET_VERSION", dataset.ID)
		if err != nil {
			return nil, err
		}
		add(items)
	}
	for _, execution := range trace.Executions {
		items, err := r.evidenceRepo.ListForObject(ctx, "EXECUTION", execution.ID)
		if err != nil {
			return nil, err
		}
		add(items)
	}
	for _, job := range trace.EntityMatchJobs {
		items, err := r.evidenceRepo.ListForObject(ctx, "ENTITY_MATCH_JOB", job.ID)
		if err != nil {
			return nil, err
		}
		add(items)
	}
	for _, mapping := range trace.EntityMappings {
		if mapping.EvidenceID == nil {
			continue
		}
		item, err := r.evidenceRepo.Get(ctx, *mapping.EvidenceID)
		if err != nil {
			return nil, err
		}
		item.RelationType = "ENTITY_MAPPING_EVIDENCE"
		add([]evidence.Item{item})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID.String() < result[j].ID.String()
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (r *Repository) collectCosts(ctx context.Context, executions []ExecutionTrace) ([]cost.Item, error) {
	result := make([]cost.Item, 0)
	for _, execution := range executions {
		items, err := r.costRepo.ListByExecution(ctx, execution.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, items...)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].OccurredAt.Before(result[j].OccurredAt) })
	return result, nil
}

func (r *Repository) collectAudit(ctx context.Context, trace ReleaseTrace) ([]AuditEventTrace, error) {
	objectIDs := []uuid.UUID{trace.ReleaseID}
	for _, execution := range trace.Executions {
		objectIDs = append(objectIDs, execution.ID)
	}
	for _, job := range trace.EntityMatchJobs {
		objectIDs = append(objectIDs, job.ID)
	}
	for _, mapping := range trace.EntityMappings {
		objectIDs = append(objectIDs, mapping.DecisionID)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, action, object_type, object_id, actor_type, actor_id,
		       COALESCE(reason,''), metadata, occurred_at
		FROM audit_event
		WHERE object_id=ANY($1::uuid[])
		ORDER BY occurred_at, id
	`, objectIDs)
	if err != nil {
		return nil, fmt.Errorf("query release audit events: %w", err)
	}
	defer rows.Close()
	result := make([]AuditEventTrace, 0)
	for rows.Next() {
		var item AuditEventTrace
		var metadata []byte
		if err := rows.Scan(
			&item.ID, &item.Action, &item.ObjectType, &item.ObjectID, &item.ActorType,
			&item.ActorID, &item.Reason, &metadata, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan release audit event: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode release audit metadata: %w", err)
			}
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
