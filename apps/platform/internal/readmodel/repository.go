package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("read model object not found")

type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type List[T any] struct {
	Items []T  `json:"items"`
	Page  Page `json:"page"`
}

type WorkbenchSummary struct {
	WorkspaceID uuid.UUID        `json:"workspaceId"`
	Counts      WorkbenchCounts  `json:"counts"`
	ReviewQueue ReviewQueueStats `json:"reviewQueue"`
	Executions  ExecutionStats   `json:"executions"`
	Releases    ReleaseStats     `json:"releases"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

type WorkbenchCounts struct {
	DataResources int `json:"dataResources"`
	Datasets      int `json:"datasets"`
	DataProducts  int `json:"dataProducts"`
}

type ReviewQueueStats struct {
	Pending    int `json:"pending"`
	Unresolved int `json:"unresolved"`
	Conflicts  int `json:"conflicts"`
}

type ExecutionStats struct {
	Queued     int `json:"queued"`
	Submitting int `json:"submitting"`
	Running    int `json:"running"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
}

type ReleaseStats struct {
	Draft     int `json:"draft"`
	Ready     int `json:"ready"`
	Published int `json:"published"`
	Failed    int `json:"failed"`
}

type DataResource struct {
	ID               uuid.UUID  `json:"id"`
	WorkspaceID      uuid.UUID  `json:"workspaceId"`
	ProjectID        *uuid.UUID `json:"projectId,omitempty"`
	Code             string     `json:"code"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	DomainCode       string     `json:"domainCode"`
	ResourceType     string     `json:"resourceType"`
	OwnerID          *uuid.UUID `json:"ownerId,omitempty"`
	SensitivityLevel string     `json:"sensitivityLevel"`
	RightsStatus     string     `json:"rightsStatus"`
	QualityStatus    string     `json:"qualityStatus"`
	LifecycleStatus  string     `json:"lifecycleStatus"`
	Revision         int64      `json:"revision"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type Dataset struct {
	ID               uuid.UUID  `json:"id"`
	WorkspaceID      uuid.UUID  `json:"workspaceId"`
	ProjectID        *uuid.UUID `json:"projectId,omitempty"`
	Code             string     `json:"code"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	DatasetType      string     `json:"datasetType"`
	SourceResourceID *uuid.UUID `json:"sourceResourceId,omitempty"`
	OwnerID          *uuid.UUID `json:"ownerId,omitempty"`
	CurrentVersionID *uuid.UUID `json:"currentVersionId,omitempty"`
	LifecycleStatus  string     `json:"lifecycleStatus"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type DatasetVersion struct {
	ID                     uuid.UUID  `json:"id"`
	DatasetID              uuid.UUID  `json:"datasetId"`
	VersionNo              int64      `json:"versionNo"`
	Status                 string     `json:"status"`
	SchemaVersion          string     `json:"schemaVersion"`
	StorageType            string     `json:"storageType"`
	StorageURI             string     `json:"storageUri"`
	ContentType            string     `json:"contentType"`
	RowCount               *int64     `json:"rowCount,omitempty"`
	ByteSize               *int64     `json:"byteSize,omitempty"`
	ChecksumAlgorithm      string     `json:"checksumAlgorithm"`
	Checksum               string     `json:"checksum"`
	GeneratedByExecutionID *uuid.UUID `json:"generatedByExecutionId,omitempty"`
	RightsSnapshotID       *uuid.UUID `json:"rightsSnapshotId,omitempty"`
	CreatedAt              time.Time  `json:"createdAt"`
	ReadyAt                *time.Time `json:"readyAt,omitempty"`
	InvalidatedAt          *time.Time `json:"invalidatedAt,omitempty"`
	InvalidationReason     string     `json:"invalidationReason"`
}

type Execution struct {
	ID                     uuid.UUID  `json:"id"`
	WorkspaceID            uuid.UUID  `json:"workspaceId"`
	WorkflowVersionID      uuid.UUID  `json:"workflowVersionId"`
	WorkflowCode           string     `json:"workflowCode"`
	WorkflowName           string     `json:"workflowName"`
	WorkflowVersion        string     `json:"workflowVersion"`
	OutputDatasetID        uuid.UUID  `json:"outputDatasetId"`
	OutputDatasetVersionID *uuid.UUID `json:"outputDatasetVersionId,omitempty"`
	TargetPeriod           string     `json:"targetPeriod"`
	Status                 string     `json:"status"`
	Attempt                int        `json:"attempt"`
	EngineType             string     `json:"engineType"`
	ErrorCode              string     `json:"errorCode"`
	CreatedAt              time.Time  `json:"createdAt"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
}

type EntityReview struct {
	CandidateID       uuid.UUID       `json:"candidateId"`
	JobID             uuid.UUID       `json:"jobId"`
	WorkspaceID       uuid.UUID       `json:"workspaceId"`
	SourceKey         string          `json:"sourceKey"`
	SourceName        string          `json:"sourceName"`
	Source            json.RawMessage `json:"source"`
	Normalized        json.RawMessage `json:"normalized"`
	CandidateEntityID *uuid.UUID      `json:"candidateEntityId,omitempty"`
	Alternatives       json.RawMessage `json:"alternatives"`
	Decision          string          `json:"decision"`
	Status            string          `json:"status"`
	// CurrentMappingDecisionID is the mapping decision that is current for this
	// candidate's source triple right now. The review form carries it as the
	// optimistic-concurrency token so a reviewer can never replace a decision
	// that changed after the queue was rendered.
	CurrentMappingDecisionID *uuid.UUID `json:"currentMappingDecisionId,omitempty"`
	MatchMethod              string     `json:"matchMethod"`
	MatchRuleID              string     `json:"matchRuleId"`
	Confidence               float64    `json:"confidence"`
	EngineName               string     `json:"engineName"`
	EngineVersion            string     `json:"engineVersion"`
	ModelVersion             string     `json:"modelVersion"`
	PolicyRef                string     `json:"policyRef"`
	PolicyVersion            string     `json:"policyVersion"`
	CreatedAt                time.Time  `json:"createdAt"`
}

type DataProduct struct {
	ID               uuid.UUID  `json:"id"`
	WorkspaceID      uuid.UUID  `json:"workspaceId"`
	ProjectID        *uuid.UUID `json:"projectId,omitempty"`
	UseCaseID        *uuid.UUID `json:"useCaseId,omitempty"`
	Code             string     `json:"code"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	DomainCode       string     `json:"domainCode"`
	LifecycleStatus  string     `json:"lifecycleStatus"`
	HealthStatus     string     `json:"healthStatus"`
	CurrentVersionID *uuid.UUID `json:"currentVersionId,omitempty"`
	CurrentVersion   string     `json:"currentVersion"`
	LatestReleaseID  *uuid.UUID `json:"latestReleaseId,omitempty"`
	LatestReleaseNo  string     `json:"latestReleaseNo"`
	LatestStatus     string     `json:"latestReleaseStatus"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type ProductRelease struct {
	ID                 uuid.UUID  `json:"id"`
	ProductID          uuid.UUID  `json:"productId"`
	ProductVersionID   uuid.UUID  `json:"productVersionId"`
	ReleaseNo          string     `json:"releaseNo"`
	Status             string     `json:"status"`
	ContractVersionID  *uuid.UUID `json:"contractVersionId,omitempty"`
	RightsSnapshotID   *uuid.UUID `json:"rightsSnapshotId,omitempty"`
	QualityResultID    *uuid.UUID `json:"qualityResultId,omitempty"`
	ComplianceResultID *uuid.UUID `json:"complianceResultId,omitempty"`
	EvidenceSnapshotID *uuid.UUID `json:"evidenceSnapshotId,omitempty"`
	ReleaseNotes       string     `json:"releaseNotes"`
	CreatedAt          time.Time  `json:"createdAt"`
	ReleasedAt         *time.Time `json:"releasedAt,omitempty"`
}

type GoldExplanation struct {
	WorkspaceID                      uuid.UUID               `json:"workspaceId"`
	OutputDatasetVersionID           uuid.UUID               `json:"outputDatasetVersionId"`
	InputDatasetVersionID            uuid.UUID               `json:"inputDatasetVersionId"`
	InputCertificationID             uuid.UUID               `json:"inputCertificationId"`
	ExecutionID                      uuid.UUID               `json:"executionId"`
	GoldProductionBindingID          uuid.UUID               `json:"goldProductionBindingId"`
	GoldProductionBindingRootHash    string                  `json:"goldProductionBindingRootHash"`
	AnnotationContributionResourceID uuid.UUID               `json:"annotationContributionResourceId"`
	Campaign                         GoldCampaignExplanation `json:"campaign"`
	Snapshot                         GoldSnapshotExplanation `json:"snapshot"`
	Reviews                          []GoldReviewExplanation `json:"reviews"`
	Trace                            GoldTraceSummary        `json:"trace"`
}

type GoldTraceSummary struct {
	Costs    []GoldCostReference     `json:"costs"`
	Evidence []GoldEvidenceReference `json:"evidence"`
	Audit    []GoldAuditReference    `json:"audit"`
}

type GoldCostReference struct {
	ID          uuid.UUID `json:"id"`
	Phase       string    `json:"phase"`
	SubjectType string    `json:"subjectType"`
	SubjectID   uuid.UUID `json:"subjectId"`
	CostType    string    `json:"costType"`
	Quantity    float64   `json:"quantity"`
	Unit        string    `json:"unit"`
	Amount      *float64  `json:"amount,omitempty"`
	Currency    string    `json:"currency,omitempty"`
	PricingMode string    `json:"pricingMode"`
	OccurredAt  time.Time `json:"occurredAt"`
}

type GoldEvidenceReference struct {
	ID            uuid.UUID  `json:"id"`
	Phase         string     `json:"phase"`
	SubjectType   string     `json:"subjectType"`
	SubjectID     uuid.UUID  `json:"subjectId"`
	EvidenceType  string     `json:"evidenceType"`
	RelationType  string     `json:"relationType"`
	SourceType    string     `json:"sourceType,omitempty"`
	SourceID      *uuid.UUID `json:"sourceId,omitempty"`
	HashAlgorithm string     `json:"hashAlgorithm,omitempty"`
	HashValue     string     `json:"hashValue,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	CreatedBy     *uuid.UUID `json:"createdBy,omitempty"`
}

type GoldAuditReference struct {
	ID         uuid.UUID  `json:"id"`
	Phase      string     `json:"phase"`
	ObjectType string     `json:"objectType"`
	ObjectID   uuid.UUID  `json:"objectId"`
	Action     string     `json:"action"`
	ActorType  string     `json:"actorType"`
	ActorID    *uuid.UUID `json:"actorId,omitempty"`
	TraceID    string     `json:"traceId,omitempty"`
	OccurredAt time.Time  `json:"occurredAt"`
}

type GoldCampaignExplanation struct {
	ID                  uuid.UUID  `json:"id"`
	Status              string     `json:"status"`
	Purpose             string     `json:"purpose"`
	Action              string     `json:"action"`
	SchemaRef           string     `json:"schemaRef"`
	SchemaVersion       string     `json:"schemaVersion"`
	SchemaSHA256        string     `json:"schemaSha256"`
	TaxonomyRef         string     `json:"taxonomyRef"`
	TaxonomyVersion     string     `json:"taxonomyVersion"`
	TaxonomySHA256      string     `json:"taxonomySha256"`
	ExpectedTaskCount   int        `json:"expectedTaskCount"`
	TaskCount           int        `json:"taskCount"`
	ResultCount         int        `json:"resultCount"`
	ReviewDecisionCount int        `json:"reviewDecisionCount"`
	SelectedOutputCount int        `json:"selectedOutputCount"`
	CreatedAt           time.Time  `json:"createdAt"`
	ActivatedAt         *time.Time `json:"activatedAt,omitempty"`
}

type GoldSnapshotExplanation struct {
	ID                    uuid.UUID  `json:"id"`
	Status                string     `json:"status"`
	RootHash              string     `json:"rootHash"`
	ExpectedTaskCount     int        `json:"expectedTaskCount"`
	ExpectedResultCount   int        `json:"expectedResultCount"`
	ExpectedDecisionCount int        `json:"expectedDecisionCount"`
	ExpectedOutputCount   int        `json:"expectedOutputCount"`
	FinalizedAt           *time.Time `json:"finalizedAt,omitempty"`
}

type GoldReviewExplanation struct {
	TaskID               uuid.UUID  `json:"taskId"`
	SourceItemRef        string     `json:"sourceItemRef"`
	SourceContentSHA256  string     `json:"sourceContentSha256"`
	DecisionID           uuid.UUID  `json:"decisionId"`
	Outcome              string     `json:"outcome"`
	ReviewerRef          string     `json:"reviewerRef"`
	Reason               string     `json:"reason"`
	ReviewedResultID     *uuid.UUID `json:"reviewedResultId,omitempty"`
	SelectedResultID     *uuid.UUID `json:"selectedResultId,omitempty"`
	SelectedResultSHA256 string     `json:"selectedResultSha256,omitempty"`
	AnnotationAuthorRef  string     `json:"annotationAuthorRef,omitempty"`
	ProviderBindingRef   string     `json:"providerBindingRef,omitempty"`
	ExternalTaskID       string     `json:"externalTaskId,omitempty"`
	ExternalAnnotationID string     `json:"externalAnnotationId,omitempty"`
	DecisionCreatedAt    time.Time  `json:"decisionCreatedAt"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GoldExplanation(
	ctx context.Context,
	workspaceID, outputDatasetVersionID uuid.UUID,
) (GoldExplanation, error) {
	var result GoldExplanation
	err := r.pool.QueryRow(ctx, `
		SELECT
			g.workspace_id,
			g.output_dataset_version_id,
			g.input_dataset_version_id,
			g.input_certification_id,
			g.execution_id,
			g.id,
			g.root_hash,
			g.annotation_contribution_resource_id,
			c.id,
			c.status,
			c.purpose,
			c.action,
			c.schema_ref,
			c.schema_version,
			c.schema_content_sha256,
			c.taxonomy_ref,
			c.taxonomy_version,
			c.taxonomy_content_sha256,
			COALESCE(c.expected_task_count,0),
			(SELECT count(*) FROM annotation_task t WHERE t.campaign_id=c.id),
			(SELECT count(*) FROM annotation_result ar WHERE ar.campaign_id=c.id),
			(SELECT count(*) FROM annotation_review_decision d WHERE d.campaign_id=c.id),
			(SELECT count(*) FROM annotation_snapshot_output so WHERE so.snapshot_id=s.id),
			c.created_at,
			c.activated_at,
			s.id,
			s.status,
			s.root_hash,
			s.expected_task_count,
			s.expected_result_count,
			s.expected_decision_count,
			s.expected_output_count,
			s.finalized_at
		FROM gold_production_binding g
		JOIN annotation_campaign c ON c.id=g.annotation_campaign_id
		JOIN annotation_snapshot s ON s.id=g.annotation_snapshot_id
		WHERE g.workspace_id=$1
		  AND g.output_dataset_version_id=$2
		  AND g.status='FINALIZED'
	`, workspaceID, outputDatasetVersionID).Scan(
		&result.WorkspaceID,
		&result.OutputDatasetVersionID,
		&result.InputDatasetVersionID,
		&result.InputCertificationID,
		&result.ExecutionID,
		&result.GoldProductionBindingID,
		&result.GoldProductionBindingRootHash,
		&result.AnnotationContributionResourceID,
		&result.Campaign.ID,
		&result.Campaign.Status,
		&result.Campaign.Purpose,
		&result.Campaign.Action,
		&result.Campaign.SchemaRef,
		&result.Campaign.SchemaVersion,
		&result.Campaign.SchemaSHA256,
		&result.Campaign.TaxonomyRef,
		&result.Campaign.TaxonomyVersion,
		&result.Campaign.TaxonomySHA256,
		&result.Campaign.ExpectedTaskCount,
		&result.Campaign.TaskCount,
		&result.Campaign.ResultCount,
		&result.Campaign.ReviewDecisionCount,
		&result.Campaign.SelectedOutputCount,
		&result.Campaign.CreatedAt,
		&result.Campaign.ActivatedAt,
		&result.Snapshot.ID,
		&result.Snapshot.Status,
		&result.Snapshot.RootHash,
		&result.Snapshot.ExpectedTaskCount,
		&result.Snapshot.ExpectedResultCount,
		&result.Snapshot.ExpectedDecisionCount,
		&result.Snapshot.ExpectedOutputCount,
		&result.Snapshot.FinalizedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoldExplanation{}, ErrNotFound
	}
	if err != nil {
		return GoldExplanation{}, fmt.Errorf("read Gold explanation: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			st.task_id,
			st.source_item_ref,
			st.source_content_sha256,
			sd.decision_id,
			sd.outcome,
			sd.reviewer_ref,
			sd.reason,
			sd.reviewed_result_id,
			sd.selected_result_id,
			COALESCE(sr.canonical_payload_sha256,''),
			COALESCE(rr.author_ref,''),
			COALESCE(rr.provider_binding_ref,''),
			COALESCE(rr.external_task_id,''),
			COALESCE(rr.external_annotation_id,''),
			d.created_at
		FROM annotation_snapshot_task st
		JOIN annotation_snapshot_decision sd
		  ON sd.snapshot_id=st.snapshot_id AND sd.task_id=st.task_id
		JOIN annotation_review_decision d ON d.id=sd.decision_id
		LEFT JOIN annotation_result rr ON rr.id=sd.reviewed_result_id
		LEFT JOIN annotation_result sr ON sr.id=sd.selected_result_id
		WHERE st.snapshot_id=$1
		ORDER BY st.source_item_ref, st.task_id
	`, result.Snapshot.ID)
	if err != nil {
		return GoldExplanation{}, fmt.Errorf("read Gold explanation reviews: %w", err)
	}
	defer rows.Close()
	result.Reviews = make([]GoldReviewExplanation, 0)
	for rows.Next() {
		var item GoldReviewExplanation
		if err := rows.Scan(
			&item.TaskID,
			&item.SourceItemRef,
			&item.SourceContentSHA256,
			&item.DecisionID,
			&item.Outcome,
			&item.ReviewerRef,
			&item.Reason,
			&item.ReviewedResultID,
			&item.SelectedResultID,
			&item.SelectedResultSHA256,
			&item.AnnotationAuthorRef,
			&item.ProviderBindingRef,
			&item.ExternalTaskID,
			&item.ExternalAnnotationID,
			&item.DecisionCreatedAt,
		); err != nil {
			return GoldExplanation{}, fmt.Errorf("scan Gold explanation review: %w", err)
		}
		result.Reviews = append(result.Reviews, item)
	}
	if err := rows.Err(); err != nil {
		return GoldExplanation{}, fmt.Errorf("iterate Gold explanation reviews: %w", err)
	}

	trace, err := r.goldTraceSummary(ctx, result)
	if err != nil {
		return GoldExplanation{}, err
	}
	result.Trace = trace
	return result, nil
}

func (r *Repository) goldTraceSummary(ctx context.Context, explanation GoldExplanation) (GoldTraceSummary, error) {
	var result GoldTraceSummary
	var err error
	result.Costs, err = r.goldCostReferences(ctx, explanation)
	if err != nil {
		return GoldTraceSummary{}, err
	}
	result.Evidence, err = r.goldEvidenceReferences(ctx, explanation)
	if err != nil {
		return GoldTraceSummary{}, err
	}
	result.Audit, err = r.goldAuditReferences(ctx, explanation)
	if err != nil {
		return GoldTraceSummary{}, err
	}
	return result, nil
}

func (r *Repository) goldCostReferences(ctx context.Context, explanation GoldExplanation) ([]GoldCostReference, error) {
	rows, err := r.pool.Query(ctx, `
		WITH subjects AS (
			SELECT 'ANNOTATION_ENGINE'::text AS phase, 'ANNOTATION_ENGINE_ATTEMPT'::text AS subject_type,
			       a.id AS subject_id, ca.cost_event_id
			FROM annotation_engine_attempt a
			JOIN annotation_engine_operation o ON o.id=a.operation_id
			JOIN cost_allocation ca ON ca.annotation_engine_attempt_id=a.id
			WHERE o.campaign_id=$1
			UNION ALL
			SELECT 'HUMAN_REVIEW', 'ANNOTATION_REVIEW_ATTEMPT',
			       a.id, ca.cost_event_id
			FROM annotation_review_attempt a
			JOIN cost_allocation ca ON ca.annotation_review_attempt_id=a.id
			WHERE a.campaign_id=$1
			UNION ALL
			SELECT 'GOLD_BUILD', 'EXECUTION',
			       $2::uuid, e.id
			FROM cost_event e
			WHERE e.execution_id=$2
			UNION ALL
			SELECT 'GOLD_QUALITY', 'QUALITY_RESULT',
			       q.id, ca.cost_event_id
			FROM quality_result q
			JOIN cost_allocation ca
			  ON ca.quality_assessment_id=q.id
			WHERE q.dataset_version_id=$3
			  AND q.rule_set_ref='gold/quality/annotation-v1'
			UNION ALL
			SELECT 'GOLD_QUALITY', 'QUALITY_RESULT',
			       q.id, ca.cost_event_id
			FROM quality_result q
			JOIN quality_assessment_attempt_outcome o
			  ON o.assessment_id=q.id AND o.outcome='SUCCEEDED'
			JOIN cost_allocation ca
			  ON ca.quality_assessment_attempt_id=o.attempt_id
			WHERE q.dataset_version_id=$3
			  AND q.rule_set_ref='gold/quality/annotation-v1'
			UNION ALL
			SELECT 'GOLD_CERTIFICATION', 'DATASET_CERTIFICATION',
			       dc.id, ca.cost_event_id
			FROM dataset_certification dc
			JOIN cost_allocation ca ON ca.dataset_certification_id=dc.id
			WHERE dc.dataset_version_id=$3
			  AND dc.profile_ref='gold/dataset-v1'
			UNION ALL
			SELECT 'DIRECT_DATA', 'DELIVERY_OPERATION',
			       d.id, ca.cost_event_id
			FROM delivery_operation d
			JOIN cost_allocation ca ON ca.delivery_operation_id=d.id
			WHERE d.dataset_version_id=$3
			  AND d.delivery_mode='DIRECT_DATA'
		)
		SELECT e.id, s.phase, s.subject_type, s.subject_id,
		       e.cost_type, e.quantity, e.unit, e.amount,
		       COALESCE(e.currency,''), e.pricing_mode, e.occurred_at
		FROM subjects s
		JOIN cost_event e ON e.id=s.cost_event_id
		WHERE e.workspace_id=$4
		ORDER BY e.occurred_at, e.id
		LIMIT 100
	`, explanation.Campaign.ID, explanation.ExecutionID, explanation.OutputDatasetVersionID, explanation.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read Gold trace costs: %w", err)
	}
	defer rows.Close()
	items := make([]GoldCostReference, 0)
	for rows.Next() {
		var item GoldCostReference
		if err := rows.Scan(
			&item.ID, &item.Phase, &item.SubjectType, &item.SubjectID,
			&item.CostType, &item.Quantity, &item.Unit, &item.Amount,
			&item.Currency, &item.PricingMode, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan Gold trace cost: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Gold trace costs: %w", err)
	}
	return items, nil
}

func (r *Repository) goldEvidenceReferences(ctx context.Context, explanation GoldExplanation) ([]GoldEvidenceReference, error) {
	rows, err := r.pool.Query(ctx, `
		WITH subjects AS (
			SELECT 'CAMPAIGN'::text AS phase, 'ANNOTATION_CAMPAIGN'::text AS object_type, $1::uuid AS object_id
			UNION ALL
			SELECT 'HUMAN_REVIEW', 'ANNOTATION_REVIEW_DECISION', d.decision_id
			FROM annotation_snapshot_decision d
			WHERE d.snapshot_id=$2
			UNION ALL
			SELECT 'GOLD_BUILD', 'EXECUTION', $3::uuid
			UNION ALL
			SELECT 'GOLD_BUILD', 'GOLD_PRODUCTION_BINDING', $4::uuid
			UNION ALL
			SELECT 'SNAPSHOT', 'ANNOTATION_SNAPSHOT', $2::uuid
			UNION ALL
			SELECT 'GOLD_BUILD', 'DATASET_VERSION', $5::uuid
			UNION ALL
			SELECT 'GOLD_QUALITY', 'QUALITY_RESULT', q.id
			FROM quality_result q
			WHERE q.dataset_version_id=$5
			  AND q.rule_set_ref='gold/quality/annotation-v1'
			UNION ALL
			SELECT 'GOLD_CERTIFICATION', 'DATASET_CERTIFICATION', dc.id
			FROM dataset_certification dc
			WHERE dc.dataset_version_id=$5
			  AND dc.profile_ref='gold/dataset-v1'
			UNION ALL
			SELECT 'DIRECT_DATA', 'DELIVERY_OPERATION', d.id
			FROM delivery_operation d
			WHERE d.dataset_version_id=$5
			  AND d.delivery_mode='DIRECT_DATA'
		)
		SELECT e.id, s.phase, s.object_type, s.object_id,
		       e.evidence_type, er.relation_type, COALESCE(e.source_type,''),
		       e.source_id, COALESCE(e.hash_algorithm,''), COALESCE(e.hash_value,''),
		       e.created_at, e.created_by
		FROM subjects s
		JOIN evidence_relation er
		  ON er.object_type=s.object_type AND er.object_id=s.object_id
		JOIN evidence e ON e.id=er.evidence_id
		WHERE e.workspace_id=$6
		ORDER BY e.created_at, e.id
		LIMIT 100
	`, explanation.Campaign.ID, explanation.Snapshot.ID, explanation.ExecutionID,
		explanation.GoldProductionBindingID, explanation.OutputDatasetVersionID, explanation.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read Gold trace evidence: %w", err)
	}
	defer rows.Close()
	items := make([]GoldEvidenceReference, 0)
	for rows.Next() {
		var item GoldEvidenceReference
		if err := rows.Scan(
			&item.ID, &item.Phase, &item.SubjectType, &item.SubjectID,
			&item.EvidenceType, &item.RelationType, &item.SourceType, &item.SourceID,
			&item.HashAlgorithm, &item.HashValue, &item.CreatedAt, &item.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan Gold trace evidence: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Gold trace evidence: %w", err)
	}
	return items, nil
}

func (r *Repository) goldAuditReferences(ctx context.Context, explanation GoldExplanation) ([]GoldAuditReference, error) {
	rows, err := r.pool.Query(ctx, `
		WITH subjects AS (
			SELECT 'CAMPAIGN'::text AS phase, 'ANNOTATION_CAMPAIGN'::text AS object_type, $1::uuid AS object_id
			UNION ALL
			SELECT 'HUMAN_REVIEW', 'ANNOTATION_REVIEW_DECISION', d.decision_id
			FROM annotation_snapshot_decision d
			WHERE d.snapshot_id=$2
			UNION ALL
			SELECT 'GOLD_BUILD', 'EXECUTION', $3::uuid
			UNION ALL
			SELECT 'GOLD_BUILD', 'GOLD_PRODUCTION_BINDING', $4::uuid
			UNION ALL
			SELECT 'SNAPSHOT', 'ANNOTATION_SNAPSHOT', $2::uuid
			UNION ALL
			SELECT 'GOLD_BUILD', 'DATASET_VERSION', $5::uuid
			UNION ALL
			SELECT 'GOLD_QUALITY', 'QUALITY_RESULT', q.id
			FROM quality_result q
			WHERE q.dataset_version_id=$5
			  AND q.rule_set_ref='gold/quality/annotation-v1'
			UNION ALL
			SELECT 'GOLD_CERTIFICATION', 'DATASET_CERTIFICATION', dc.id
			FROM dataset_certification dc
			WHERE dc.dataset_version_id=$5
			  AND dc.profile_ref='gold/dataset-v1'
			UNION ALL
			SELECT 'DIRECT_DATA', 'DELIVERY_OPERATION', d.id
			FROM delivery_operation d
			WHERE d.dataset_version_id=$5
			  AND d.delivery_mode='DIRECT_DATA'
		)
		SELECT a.id, s.phase, a.object_type, a.object_id,
		       a.action, a.actor_type, a.actor_id, COALESCE(a.trace_id,''), a.occurred_at
		FROM subjects s
		JOIN audit_event a
		  ON a.object_type=s.object_type AND a.object_id=s.object_id
		WHERE a.workspace_id=$6
		ORDER BY a.occurred_at, a.id
		LIMIT 100
	`, explanation.Campaign.ID, explanation.Snapshot.ID, explanation.ExecutionID,
		explanation.GoldProductionBindingID, explanation.OutputDatasetVersionID, explanation.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("read Gold trace audit: %w", err)
	}
	defer rows.Close()
	items := make([]GoldAuditReference, 0)
	for rows.Next() {
		var item GoldAuditReference
		if err := rows.Scan(
			&item.ID, &item.Phase, &item.ObjectType, &item.ObjectID,
			&item.Action, &item.ActorType, &item.ActorID, &item.TraceID, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan Gold trace audit: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Gold trace audit: %w", err)
	}
	return items, nil
}

func (r *Repository) Workbench(ctx context.Context, workspaceID uuid.UUID) (WorkbenchSummary, error) {
	var result WorkbenchSummary
	result.WorkspaceID = workspaceID
	result.UpdatedAt = time.Now().UTC()

	if err := r.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM data_resource WHERE workspace_id=$1 AND deleted_at IS NULL),
			(SELECT count(*) FROM dataset WHERE workspace_id=$1 AND deleted_at IS NULL),
			(SELECT count(*) FROM data_product WHERE workspace_id=$1 AND deleted_at IS NULL),
			(SELECT count(*) FROM entity_match_candidate c JOIN entity_match_job j ON j.id=c.job_id WHERE j.workspace_id=$1 AND c.status='PENDING'),
			(SELECT count(*) FROM entity_match_candidate c JOIN entity_match_job j ON j.id=c.job_id WHERE j.workspace_id=$1 AND c.status='UNRESOLVED'),
			(SELECT count(*) FROM entity_mapping m JOIN entity e ON e.id=m.entity_id WHERE e.workspace_id=$1 AND m.status='CONFLICT'),
			(SELECT count(*) FROM execution WHERE workspace_id=$1 AND status='QUEUED'),
			(SELECT count(*) FROM execution WHERE workspace_id=$1 AND status='SUBMITTING'),
			(SELECT count(*) FROM execution WHERE workspace_id=$1 AND status='RUNNING'),
			(SELECT count(*) FROM execution WHERE workspace_id=$1 AND status='SUCCEEDED'),
			(SELECT count(*) FROM execution WHERE workspace_id=$1 AND status='FAILED'),
			(SELECT count(*) FROM product_release pr JOIN data_product p ON p.id=pr.product_id WHERE p.workspace_id=$1 AND pr.status='DRAFT'),
			(SELECT count(*) FROM product_release pr JOIN data_product p ON p.id=pr.product_id WHERE p.workspace_id=$1 AND pr.status='READY'),
			(SELECT count(*) FROM product_release pr JOIN data_product p ON p.id=pr.product_id WHERE p.workspace_id=$1 AND pr.status='PUBLISHED'),
			(SELECT count(*) FROM product_release pr JOIN data_product p ON p.id=pr.product_id WHERE p.workspace_id=$1 AND pr.status='FAILED')
	`, workspaceID).Scan(
		&result.Counts.DataResources, &result.Counts.Datasets, &result.Counts.DataProducts,
		&result.ReviewQueue.Pending, &result.ReviewQueue.Unresolved, &result.ReviewQueue.Conflicts,
		&result.Executions.Queued, &result.Executions.Submitting, &result.Executions.Running,
		&result.Executions.Succeeded, &result.Executions.Failed,
		&result.Releases.Draft, &result.Releases.Ready, &result.Releases.Published, &result.Releases.Failed,
	); err != nil {
		return WorkbenchSummary{}, fmt.Errorf("read workbench summary: %w", err)
	}
	return result, nil
}

func (r *Repository) ListDataResources(ctx context.Context, workspaceID uuid.UUID, limit, offset int) (List[DataResource], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM data_resource WHERE workspace_id=$1 AND deleted_at IS NULL`, workspaceID)
	if err != nil {
		return List[DataResource]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, project_id, code, name, COALESCE(description,''), COALESCE(domain_code,''),
		       resource_type, owner_id, COALESCE(sensitivity_level,''), rights_status, quality_status,
		       lifecycle_status, revision, created_at, updated_at
		FROM data_resource
		WHERE workspace_id=$1 AND deleted_at IS NULL
		ORDER BY updated_at DESC, id
		LIMIT $2 OFFSET $3
	`, workspaceID, limit, offset)
	if err != nil {
		return List[DataResource]{}, fmt.Errorf("list data resources: %w", err)
	}
	defer rows.Close()
	items := make([]DataResource, 0)
	for rows.Next() {
		item, err := scanDataResource(rows)
		if err != nil {
			return List[DataResource]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[DataResource]{}, fmt.Errorf("iterate data resources: %w", err)
	}
	return List[DataResource]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) GetDataResource(ctx context.Context, resourceID uuid.UUID) (DataResource, error) {
	item, err := scanDataResource(r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, project_id, code, name, COALESCE(description,''), COALESCE(domain_code,''),
		       resource_type, owner_id, COALESCE(sensitivity_level,''), rights_status, quality_status,
		       lifecycle_status, revision, created_at, updated_at
		FROM data_resource WHERE id=$1 AND deleted_at IS NULL
	`, resourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return DataResource{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) ListDatasets(ctx context.Context, workspaceID uuid.UUID, limit, offset int) (List[Dataset], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM dataset WHERE workspace_id=$1 AND deleted_at IS NULL`, workspaceID)
	if err != nil {
		return List[Dataset]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, project_id, code, name, COALESCE(description,''), dataset_type,
		       source_resource_id, owner_id, current_version_id, lifecycle_status, created_at, updated_at
		FROM dataset WHERE workspace_id=$1 AND deleted_at IS NULL
		ORDER BY updated_at DESC, id LIMIT $2 OFFSET $3
	`, workspaceID, limit, offset)
	if err != nil {
		return List[Dataset]{}, fmt.Errorf("list datasets: %w", err)
	}
	defer rows.Close()
	items := make([]Dataset, 0)
	for rows.Next() {
		item, err := scanDataset(rows)
		if err != nil {
			return List[Dataset]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[Dataset]{}, fmt.Errorf("iterate datasets: %w", err)
	}
	return List[Dataset]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) GetDataset(ctx context.Context, datasetID uuid.UUID) (Dataset, error) {
	item, err := scanDataset(r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, project_id, code, name, COALESCE(description,''), dataset_type,
		       source_resource_id, owner_id, current_version_id, lifecycle_status, created_at, updated_at
		FROM dataset WHERE id=$1 AND deleted_at IS NULL
	`, datasetID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) ListDatasetVersions(ctx context.Context, datasetID uuid.UUID, limit, offset int) (List[DatasetVersion], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, datasetID)
	if err != nil {
		return List[DatasetVersion]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, dataset_id, version_no, status, COALESCE(schema_version,''), COALESCE(storage_type,''),
		       COALESCE(storage_uri,''), COALESCE(content_type,''), row_count, byte_size,
		       COALESCE(checksum_algorithm,''), COALESCE(checksum_value,''), generated_by_execution_id,
		       rights_snapshot_id, created_at, ready_at, invalidated_at, COALESCE(invalidation_reason,'')
		FROM dataset_version WHERE dataset_id=$1
		ORDER BY version_no DESC LIMIT $2 OFFSET $3
	`, datasetID, limit, offset)
	if err != nil {
		return List[DatasetVersion]{}, fmt.Errorf("list dataset versions: %w", err)
	}
	defer rows.Close()
	items := make([]DatasetVersion, 0)
	for rows.Next() {
		var item DatasetVersion
		if err := rows.Scan(
			&item.ID, &item.DatasetID, &item.VersionNo, &item.Status, &item.SchemaVersion, &item.StorageType,
			&item.StorageURI, &item.ContentType, &item.RowCount, &item.ByteSize, &item.ChecksumAlgorithm,
			&item.Checksum, &item.GeneratedByExecutionID, &item.RightsSnapshotID,
			&item.CreatedAt, &item.ReadyAt, &item.InvalidatedAt, &item.InvalidationReason,
		); err != nil {
			return List[DatasetVersion]{}, fmt.Errorf("scan dataset version: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[DatasetVersion]{}, fmt.Errorf("iterate dataset versions: %w", err)
	}
	return List[DatasetVersion]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) ListExecutions(ctx context.Context, workspaceID uuid.UUID, limit, offset int) (List[Execution], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM execution WHERE workspace_id=$1`, workspaceID)
	if err != nil {
		return List[Execution]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.workspace_id, e.workflow_version_id, w.code, w.name, wv.version,
		       e.output_dataset_id, e.output_dataset_version_id, e.target_period, e.status, e.attempt,
		       e.engine_type, COALESCE(e.error_code,''), e.created_at, e.started_at, e.finished_at
		FROM execution e
		JOIN workflow_version wv ON wv.id=e.workflow_version_id
		JOIN workflow w ON w.id=wv.workflow_id
		WHERE e.workspace_id=$1
		ORDER BY e.created_at DESC, e.id LIMIT $2 OFFSET $3
	`, workspaceID, limit, offset)
	if err != nil {
		return List[Execution]{}, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()
	items := make([]Execution, 0)
	for rows.Next() {
		var item Execution
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkflowVersionID, &item.WorkflowCode, &item.WorkflowName,
			&item.WorkflowVersion, &item.OutputDatasetID, &item.OutputDatasetVersionID, &item.TargetPeriod,
			&item.Status, &item.Attempt, &item.EngineType, &item.ErrorCode, &item.CreatedAt, &item.StartedAt, &item.FinishedAt,
		); err != nil {
			return List[Execution]{}, fmt.Errorf("scan execution: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[Execution]{}, fmt.Errorf("iterate executions: %w", err)
	}
	return List[Execution]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) ListEntityReviews(ctx context.Context, workspaceID uuid.UUID, status string, limit, offset int) (List[EntityReview], error) {
	args := []any{workspaceID}
	filter := ""
	if status != "" {
		args = append(args, status)
		filter = fmt.Sprintf(" AND c.status=$%d", len(args))
	}
	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM entity_match_candidate c
		JOIN entity_match_job j ON j.id=c.job_id
		WHERE j.workspace_id=$1`+filter, args...).Scan(&total); err != nil {
		return List[EntityReview]{}, fmt.Errorf("count entity reviews: %w", err)
	}
	args = append(args, limit, offset)
	limitPos := len(args) - 1
	offsetPos := len(args)
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.job_id, j.workspace_id, c.source_key, COALESCE(c.source_name,''), c.source_payload,
		       c.normalized_payload, c.candidate_entity_id, c.candidate_alternatives, c.decision, c.status, c.match_method,
		       COALESCE(c.match_rule_id,''), COALESCE(c.confidence,0), c.match_engine_name,
		       c.match_engine_version, c.match_model_version, j.policy_ref, j.policy_version, c.created_at,
		       em.current_decision_id
		FROM entity_match_candidate c
		JOIN entity_match_job j ON j.id=c.job_id
		LEFT JOIN entity_mapping em
		  ON em.workspace_id=j.workspace_id
		 AND em.source_type=j.source_type
		 AND em.source_ref=j.source_ref
		 AND em.source_key=c.source_key
		WHERE j.workspace_id=$1`+filter+fmt.Sprintf(`
		ORDER BY c.created_at DESC, c.id LIMIT $%d OFFSET $%d`, limitPos, offsetPos), args...)
	if err != nil {
		return List[EntityReview]{}, fmt.Errorf("list entity reviews: %w", err)
	}
	defer rows.Close()
	items := make([]EntityReview, 0)
	for rows.Next() {
		var item EntityReview
		var source, normalized, alternatives []byte
		if err := rows.Scan(
			&item.CandidateID, &item.JobID, &item.WorkspaceID, &item.SourceKey, &item.SourceName, &source,
			&normalized, &item.CandidateEntityID, &alternatives, &item.Decision, &item.Status, &item.MatchMethod,
			&item.MatchRuleID, &item.Confidence, &item.EngineName, &item.EngineVersion, &item.ModelVersion,
			&item.PolicyRef, &item.PolicyVersion, &item.CreatedAt, &item.CurrentMappingDecisionID,
		); err != nil {
			return List[EntityReview]{}, fmt.Errorf("scan entity review: %w", err)
		}
		item.Source = append(json.RawMessage(nil), source...)
		item.Normalized = append(json.RawMessage(nil), normalized...)
		item.Alternatives = append(json.RawMessage(nil), alternatives...)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[EntityReview]{}, fmt.Errorf("iterate entity reviews: %w", err)
	}
	return List[EntityReview]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) ListDataProducts(ctx context.Context, workspaceID uuid.UUID, limit, offset int) (List[DataProduct], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM data_product WHERE workspace_id=$1 AND deleted_at IS NULL`, workspaceID)
	if err != nil {
		return List[DataProduct]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.workspace_id, p.project_id, p.use_case_id, p.code, p.name, COALESCE(p.description,''),
		       COALESCE(p.domain_code,''), p.lifecycle_status, p.health_status, p.current_version_id,
		       CASE WHEN pv.id IS NULL THEN '' ELSE pv.major_version || '.' || pv.minor_version || '.' || pv.patch_version END,
		       p.latest_release_id, COALESCE(pr.release_no,''), COALESCE(pr.status,''), p.created_at, p.updated_at
		FROM data_product p
		LEFT JOIN product_version pv ON pv.id=p.current_version_id
		LEFT JOIN product_release pr ON pr.id=p.latest_release_id
		WHERE p.workspace_id=$1 AND p.deleted_at IS NULL
		ORDER BY p.updated_at DESC, p.id LIMIT $2 OFFSET $3
	`, workspaceID, limit, offset)
	if err != nil {
		return List[DataProduct]{}, fmt.Errorf("list data products: %w", err)
	}
	defer rows.Close()
	items := make([]DataProduct, 0)
	for rows.Next() {
		var item DataProduct
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.ProjectID, &item.UseCaseID, &item.Code, &item.Name,
			&item.Description, &item.DomainCode, &item.LifecycleStatus, &item.HealthStatus, &item.CurrentVersionID,
			&item.CurrentVersion, &item.LatestReleaseID, &item.LatestReleaseNo, &item.LatestStatus,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return List[DataProduct]{}, fmt.Errorf("scan data product: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[DataProduct]{}, fmt.Errorf("iterate data products: %w", err)
	}
	return List[DataProduct]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) ListProductReleases(ctx context.Context, productID uuid.UUID, limit, offset int) (List[ProductRelease], error) {
	total, err := r.count(ctx, `SELECT count(*) FROM product_release WHERE product_id=$1`, productID)
	if err != nil {
		return List[ProductRelease]{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, product_id, product_version_id, release_no, status, contract_version_id, rights_snapshot_id,
		       quality_result_id, compliance_result_id, evidence_snapshot_id, COALESCE(release_notes,''), created_at, released_at
		FROM product_release WHERE product_id=$1
		ORDER BY created_at DESC, id LIMIT $2 OFFSET $3
	`, productID, limit, offset)
	if err != nil {
		return List[ProductRelease]{}, fmt.Errorf("list product releases: %w", err)
	}
	defer rows.Close()
	items := make([]ProductRelease, 0)
	for rows.Next() {
		var item ProductRelease
		if err := rows.Scan(
			&item.ID, &item.ProductID, &item.ProductVersionID, &item.ReleaseNo, &item.Status,
			&item.ContractVersionID, &item.RightsSnapshotID, &item.QualityResultID, &item.ComplianceResultID,
			&item.EvidenceSnapshotID, &item.ReleaseNotes, &item.CreatedAt, &item.ReleasedAt,
		); err != nil {
			return List[ProductRelease]{}, fmt.Errorf("scan product release: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return List[ProductRelease]{}, fmt.Errorf("iterate product releases: %w", err)
	}
	return List[ProductRelease]{Items: items, Page: Page{Limit: limit, Offset: offset, Total: total}}, nil
}

func (r *Repository) count(ctx context.Context, query string, args ...any) (int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count read model rows: %w", err)
	}
	return total, nil
}

func scanDataResource(row pgx.Row) (DataResource, error) {
	var item DataResource
	if err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.ProjectID, &item.Code, &item.Name, &item.Description,
		&item.DomainCode, &item.ResourceType, &item.OwnerID, &item.SensitivityLevel, &item.RightsStatus,
		&item.QualityStatus, &item.LifecycleStatus, &item.Revision, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return DataResource{}, err
	}
	return item, nil
}

func scanDataset(row pgx.Row) (Dataset, error) {
	var item Dataset
	if err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.ProjectID, &item.Code, &item.Name, &item.Description,
		&item.DatasetType, &item.SourceResourceID, &item.OwnerID, &item.CurrentVersionID, &item.LifecycleStatus,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return Dataset{}, err
	}
	return item, nil
}
