package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"strconv"
	"time"
)

type Quality interface {
	Run(context.Context, qualityapp.RunCommand) (qualitydomain.Assessment, error)
}
type Service struct {
	Tx         *transaction.Manager
	Repo       *infrastructure.Repository
	Quality    Quality
	Executions *workflowapp.ExecutionService
	Workflows  *workflowinfra.PostgresRepository
	Plan       Plan
}
type Command struct {
	WorkspaceID, ActorID, CaseID uuid.UUID
	Key, Reason, TraceID         string
}
type ExecuteCommand struct {
	Command
	WorkflowVersionID, OutputDatasetID uuid.UUID
	TargetPeriod                       string
	Inputs                             []workflowdomain.InputBinding
	Retry                              bool
}
type Detection struct {
	CaseID       *uuid.UUID `json:"caseId,omitempty"`
	AssessmentID uuid.UUID  `json:"assessmentId"`
	Decision     string     `json:"decision"`
}

func (s *Service) Detect(ctx context.Context, cmd Command, version uuid.UUID) (Detection, error) {
	var result Detection
	if err := domain.ValidateCommand(cmd.WorkspaceID, cmd.ActorID, cmd.Key); err != nil {
		return result, err
	}
	if version == uuid.Nil || s.Plan.Hash == "" {
		return result, domain.ErrScope
	}
	var detectionID uuid.UUID
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_version v JOIN dataset ds ON ds.id=v.dataset_id WHERE v.id=$1 AND ds.workspace_id=$2 AND ds.dataset_type='RAW')`, version, cmd.WorkspaceID).Scan(&raw); err != nil {
			return err
		}
		if !raw {
			return domain.ErrScope
		}
		_, err := tx.Exec(ctx, `INSERT INTO quality_remediation_detection(id,workspace_id,version_id,request_key,actor_id,plan_hash)
   VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,request_key) DO NOTHING`, uuid.New(), cmd.WorkspaceID, version, cmd.Key, cmd.ActorID, s.Plan.Hash)
		if err != nil {
			return err
		}
		var input, actor uuid.UUID
		var hash string
		err = tx.QueryRow(ctx, `SELECT id,version_id,actor_id,plan_hash FROM quality_remediation_detection WHERE workspace_id=$1 AND request_key=$2 FOR UPDATE`, cmd.WorkspaceID, cmd.Key).Scan(&detectionID, &input, &actor, &hash)
		if err != nil {
			return err
		}
		if input != version || actor != cmd.ActorID || hash != s.Plan.Hash {
			return domain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	assessment, err := s.Quality.Run(ctx, qualityapp.RunCommand{WorkspaceID: cmd.WorkspaceID, DatasetVersionID: version, RuleSetRef: s.Plan.Spec.DetectionRuleSetRef, AssessmentAttemptID: detectionID, ActorID: &cmd.ActorID, TraceID: cmd.TraceID, Now: time.Now().UTC()})
	if err != nil {
		return result, err
	}
	result.AssessmentID, result.Decision = assessment.ID, string(assessment.GateDecision)
	if assessment.RuleSetContentSHA256 != s.Plan.DetectionHash {
		return result, domain.ErrEvidence
	}
	var failed *qualitydomain.Finding
	formatValid := false
	for i := range assessment.Findings {
		f := &assessment.Findings[i]
		if f.RuleID == s.Plan.Spec.DetectionFormatRuleID && f.Status == qualitydomain.FindingPass {
			formatValid = true
		}
		if f.RuleID == s.Plan.Spec.DetectionRuleID && f.Status == qualitydomain.FindingFail {
			failed = f
		}
	}
	if failed == nil || !formatValid {
		return result, nil
	}
	invalid, e := strconv.ParseInt(fmt.Sprint(failed.Observed["affectedCount"]), 10, 64)
	total, totalErr := strconv.ParseInt(fmt.Sprint(failed.Observed["total"]), 10, 64)
	if e != nil || invalid < 1 || totalErr != nil || total < invalid {
		return result, domain.ErrEvidence
	}
	err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM quality_remediation_detection WHERE id=$1 FOR UPDATE`, detectionID).Scan(&locked); err != nil {
			return err
		}
		var existing uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM quality_remediation_case WHERE id=$1`, detectionID).Scan(&existing)
		if err == nil {
			result.CaseID = &existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO quality_remediation_case(id,workspace_id,version_id,assessment_id,finding_id,plan_ref,plan_version,plan_hash,
   detection_rule_id,workflow_hash,input_name,quarantine_reason,recheck_rule_ref,recheck_rule_hash,created_by)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, detectionID, cmd.WorkspaceID, version, assessment.ID, failed.ID, s.Plan.Ref, s.Plan.Metadata.Version, s.Plan.Hash,
			s.Plan.Spec.DetectionRuleID, s.Plan.WorkflowHash, s.Plan.Spec.InputName, s.Plan.Spec.QuarantineReasonCode, s.Plan.Spec.RecheckRuleSetRef, s.Plan.RecheckHash, cmd.ActorID)
		if err != nil {
			return err
		}
		result.CaseID = &detectionID
		return proof(ctx, tx, cmd, detectionID, "QualityRemediationDetected", map[string]any{"versionId": version, "assessmentId": assessment.ID, "findingId": failed.ID, "planHash": s.Plan.Hash})
	})
	return result, err
}

func (s *Service) lock(ctx context.Context, tx pgx.Tx, cmd Command, needsAssignee, recoverAccepted bool) (domain.Case, uuid.UUID, error) {
	c, err := s.Repo.CaseTx(ctx, tx, cmd.WorkspaceID, cmd.CaseID)
	if err != nil {
		return c, uuid.Nil, err
	}
	var closed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quality_remediation_close WHERE case_id=$1)`, c.ID).Scan(&closed); err != nil {
		return c, uuid.Nil, err
	}
	if closed && !recoverAccepted {
		return c, uuid.Nil, domain.ErrClosed
	}
	var assignment, actor uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id,actor_id FROM quality_remediation_assignment WHERE case_id=$1 ORDER BY ordinal DESC LIMIT 1`, c.ID).Scan(&assignment, &actor)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, uuid.Nil, err
	}
	if needsAssignee {
		if e := domain.CanHandle(false, actor, cmd.ActorID); e != nil {
			return c, uuid.Nil, e
		}
	}
	return c, assignment, nil
}

func (s *Service) Assign(ctx context.Context, cmd Command) (uuid.UUID, error) {
	var id uuid.UUID
	if err := domain.ValidateCommand(cmd.WorkspaceID, cmd.ActorID, cmd.Key); err != nil {
		return id, err
	}
	if err := domain.ValidateReason(cmd.Reason); err != nil {
		return id, err
	}
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, _, err := s.lock(ctx, tx, cmd, false, true)
		if err != nil {
			return err
		}
		var actor uuid.UUID
		var reason string
		err = tx.QueryRow(ctx, `SELECT id,actor_id,reason FROM quality_remediation_assignment WHERE case_id=$1 AND request_key=$2`, cmd.CaseID, cmd.Key).Scan(&id, &actor, &reason)
		if err == nil {
			if actor != cmd.ActorID || reason != cmd.Reason {
				return domain.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, _, err = s.lock(ctx, tx, cmd, false, false); err != nil {
			return err
		}
		id = uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO quality_remediation_assignment(id,case_id,actor_id,reason,request_key,ordinal)
   SELECT $1,$2,$3,$4,$5,count(*)+1 FROM quality_remediation_assignment WHERE case_id=$2`, id, cmd.CaseID, cmd.ActorID, cmd.Reason, cmd.Key)
		if err != nil {
			return err
		}
		return proof(ctx, tx, cmd, cmd.CaseID, "QualityRemediationAssigned", map[string]any{"assignmentId": id, "actorId": cmd.ActorID})
	})
	return id, err
}

func (s *Service) Execute(ctx context.Context, cmd ExecuteCommand) (workflowdomain.Execution, error) {
	var result workflowdomain.Execution
	if err := domain.ValidateCommand(cmd.WorkspaceID, cmd.ActorID, cmd.Key); err != nil {
		return result, err
	}
	if err := domain.ValidateReason(cmd.Reason); err != nil {
		return result, err
	}
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		c, _, err := s.lock(ctx, tx, cmd.Command, false, true)
		if err != nil {
			return err
		}
		var priorID uuid.UUID
		var priorReason string
		var priorActor uuid.UUID
		err = tx.QueryRow(ctx, `SELECT execution_id,reason,actor_id FROM quality_remediation_execution WHERE case_id=$1 AND request_key=$2`, c.ID, cmd.Key).Scan(&priorID, &priorReason, &priorActor)
		if err == nil {
			if priorReason != cmd.Reason || priorActor != cmd.ActorID {
				return domain.ErrConflict
			}
			result, err = s.Workflows.GetExecutionTx(ctx, tx, priorID, false)
			if err != nil {
				return err
			}
			if cmd.Retry {
				if result.RetryOfExecutionID == nil {
					return domain.ErrConflict
				}
			} else {
				if result.RetryOfExecutionID != nil || !sameInputs(result.Inputs, cmd.Inputs) || result.WorkflowVersionID != cmd.WorkflowVersionID || result.OutputDatasetID != cmd.OutputDatasetID || result.TargetPeriod != cmd.TargetPeriod {
					return domain.ErrConflict
				}
			}
			return s.Workflows.ValidateExecutionAccess(ctx, tx, result)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, assignment, err := s.lock(ctx, tx, cmd.Command, true, false)
		if err != nil {
			return err
		}
		if c.PlanHash != s.Plan.Hash || c.WorkflowHash != s.Plan.WorkflowHash {
			return domain.ErrScope
		}
		// The versioned server-side plan owns the destination binding. Caller
		// input cannot select another asset, even in the same workspace.
		var approvedOutput uuid.UUID
		var outputType string
		if err = tx.QueryRow(ctx, `SELECT id,dataset_type FROM dataset WHERE workspace_id=$1 AND code=$2 FOR SHARE`, c.WorkspaceID, s.Plan.Spec.OutputDatasetCode).Scan(&approvedOutput, &outputType); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrScope
			}
			return err
		}
		if outputType != "CURATED" || (!cmd.Retry && cmd.OutputDatasetID != approvedOutput) {
			return domain.ErrScope
		}
		var previous uuid.UUID
		var ordinal int
		err = tx.QueryRow(ctx, `SELECT execution_id,ordinal FROM quality_remediation_execution WHERE case_id=$1 ORDER BY ordinal DESC LIMIT 1`, c.ID).Scan(&previous, &ordinal)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if cmd.Retry {
			if previous == uuid.Nil {
				return domain.ErrAttempt
			}
			prior, e := s.Workflows.GetExecutionTx(ctx, tx, previous, false)
			if e != nil {
				return e
			}
			if prior.OutputDatasetID != approvedOutput {
				return domain.ErrScope
			}
			if e = s.validateInputs(ctx, tx, c, prior.Inputs); e != nil {
				return e
			}
			result, err = s.Executions.RetryInTransaction(ctx, tx, workflowapp.RetryExecutionCommand{ExecutionID: previous, IdempotencyKey: executionKey(c.ID, cmd.Key), ActorID: &cmd.ActorID, TraceID: cmd.TraceID})
		} else {
			if previous != uuid.Nil {
				return domain.ErrAttempt
			}
			var hash string
			if err = tx.QueryRow(ctx, `SELECT definition_sha256 FROM workflow_version WHERE id=$1`, cmd.WorkflowVersionID).Scan(&hash); err != nil {
				return err
			}
			if hash != c.WorkflowHash {
				return domain.ErrScope
			}
			if err = s.validateInputs(ctx, tx, c, cmd.Inputs); err != nil {
				return err
			}
			result, err = s.Executions.CreateInTransaction(ctx, tx, workflowapp.CreateExecutionCommand{WorkspaceID: cmd.WorkspaceID, WorkflowVersionID: cmd.WorkflowVersionID, OutputDatasetID: cmd.OutputDatasetID, TargetPeriod: cmd.TargetPeriod, Inputs: cmd.Inputs, IdempotencyKey: executionKey(c.ID, cmd.Key), ActorID: &cmd.ActorID, TraceID: cmd.TraceID})
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO quality_remediation_execution(id,case_id,execution_id,assignment_id,ordinal,actor_id,request_key,reason)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.New(), c.ID, result.ID, assignment, ordinal+1, cmd.ActorID, cmd.Key, cmd.Reason)
		if err != nil {
			return err
		}
		return proof(ctx, tx, cmd.Command, c.ID, "QualityRemediationExecutionAccepted", map[string]any{"executionId": result.ID, "attempt": ordinal + 1})
	})
	return result, err
}

// Accepted receipt membership and the resolution owner's actual output bind
// every workflow input. The generic Execution service cannot decide C1's batch
// semantics; missing or substituted inputs must fail before costing engine work.
func (s *Service) validateInputs(ctx context.Context, tx pgx.Tx, c domain.Case, inputs []workflowdomain.InputBinding) error {
	return s.validateInputProvenance(ctx, tx, c, inputs, nil, nil)
}

// validateInputProvenance is the existing owner rule, also used by the Agent's
// read-only plan preparation. Collecting references does not run any Command.
func (s *Service) validateInputProvenance(ctx context.Context, tx pgx.Tx, c domain.Case, inputs []workflowdomain.InputBinding, receiptOut *uuid.UUID, resolutionOut map[string]uuid.UUID) error {
	if len(inputs) != len(s.Plan.Inputs) {
		return domain.ErrScope
	}
	provided := map[string]uuid.UUID{}
	for _, input := range inputs {
		if input.DatasetVersionID == uuid.Nil || provided[input.Name] != uuid.Nil {
			return domain.ErrScope
		}
		provided[input.Name] = input.DatasetVersionID
	}
	var receipt uuid.UUID
	err := tx.QueryRow(ctx, `SELECT r.id FROM ingestion_receipt_slot b JOIN ingestion_receipt r ON r.id=b.receipt_id JOIN ingestion_operation o ON o.id=r.operation_id WHERE b.version_id=$1 AND b.code=$2 AND o.workspace_id=$3 AND o.state='ACCEPTED'`, c.VersionID, c.InputName, c.WorkspaceID).Scan(&receipt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrScope
	}
	if err != nil {
		return err
	}
	if receiptOut != nil {
		*receiptOut = receipt
	}
	rows, err := tx.Query(ctx, `SELECT b.code,b.version_id,ds.workspace_id,ds.dataset_type FROM ingestion_receipt_slot b JOIN dataset_version v ON v.id=b.version_id JOIN dataset ds ON ds.id=v.dataset_id WHERE b.receipt_id=$1`, receipt)
	if err != nil {
		return err
	}
	approved := map[string]uuid.UUID{}
	for rows.Next() {
		var name, kind string
		var version, workspace uuid.UUID
		if err = rows.Scan(&name, &version, &workspace, &kind); err != nil {
			rows.Close()
			return err
		}
		if workspace != c.WorkspaceID || kind != "RAW" {
			rows.Close()
			return domain.ErrScope
		}
		approved[name] = version
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(approved) != 3 {
		return domain.ErrScope
	}
	for _, input := range s.Plan.Inputs {
		switch input.DatasetType {
		case "RAW":
			if approved[input.Name] == uuid.Nil || provided[input.Name] != approved[input.Name] {
				return domain.ErrScope
			}
		case "STANDARDIZED":
			var job uuid.UUID
			err = tx.QueryRow(ctx, `SELECT j.id FROM entity_match_job j JOIN dataset_version v ON v.id=j.output_dataset_version_id JOIN dataset ds ON ds.id=v.dataset_id WHERE j.workspace_id=$1 AND j.input_dataset_version_id=$2 AND j.output_dataset_version_id=$3 AND j.status='SUCCEEDED' AND v.generated_by_entity_match_job_id=j.id AND ds.workspace_id=$1 AND ds.dataset_type='STANDARDIZED' FOR SHARE OF j`, c.WorkspaceID, approved[s.Plan.Spec.ResolutionSourceInputName], provided[input.Name]).Scan(&job)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrScope
			}
			if err != nil {
				return err
			}
			if resolutionOut != nil {
				resolutionOut[input.Name] = job
			}
		default:
			return domain.ErrScope
		}
	}
	return nil
}

func sameInputs(a, b []workflowdomain.InputBinding) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]uuid.UUID{}
	for _, i := range a {
		m[i.Name] = i.DatasetVersionID
	}
	seen := map[string]bool{}
	for _, i := range b {
		if seen[i.Name] || m[i.Name] != i.DatasetVersionID {
			return false
		}
		seen[i.Name] = true
	}
	return true
}

func executionKey(id uuid.UUID, key string) string {
	return "remediation-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(id.String()+"\x00"+key)).String()
}

func (s *Service) Recheck(ctx context.Context, cmd Command, linkID uuid.UUID) (uuid.UUID, error) {
	var id, version uuid.UUID
	var c domain.Case
	if err := domain.ValidateCommand(cmd.WorkspaceID, cmd.ActorID, cmd.Key); err != nil {
		return id, err
	}
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		c, _, err = s.lock(ctx, tx, cmd, false, true)
		if err != nil {
			return err
		}
		var actor, priorLink uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id,actor_id,execution_link_id FROM quality_remediation_recheck WHERE case_id=$1 AND request_key=$2`, c.ID, cmd.Key).Scan(&id, &actor, &priorLink)
		if err == nil {
			if actor != cmd.ActorID || priorLink != linkID {
				return domain.ErrConflict
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			if _, _, err = s.lock(ctx, tx, cmd, true, false); err != nil {
				return err
			}
			if c.RecheckRuleHash != s.Plan.RecheckHash || c.PlanHash != s.Plan.Hash {
				return domain.ErrScope
			}
			id = uuid.New()
			_, err = tx.Exec(ctx, `INSERT INTO quality_remediation_recheck(id,case_id,execution_link_id,quality_attempt_id,actor_id,request_key) VALUES($1,$2,$3,$1,$4,$5)`, id, c.ID, linkID, cmd.ActorID, cmd.Key)
			if err != nil {
				return err
			}
			if err = proof(ctx, tx, cmd, c.ID, "QualityRemediationRecheckStarted", map[string]any{"recheckId": id, "executionLinkId": linkID}); err != nil {
				return err
			}
		} else {
			return err
		}
		return tx.QueryRow(ctx, `SELECT e.output_dataset_version_id FROM quality_remediation_execution l JOIN execution e ON e.id=l.execution_id WHERE l.id=$1 AND l.case_id=$2 AND e.status='SUCCEEDED'`, linkID, c.ID).Scan(&version)
	})
	if err != nil {
		return id, err
	}
	assessment, runErr := s.Quality.Run(ctx, qualityapp.RunCommand{WorkspaceID: cmd.WorkspaceID, DatasetVersionID: version, RuleSetRef: c.RecheckRuleRef, AssessmentAttemptID: id, ActorID: &cmd.ActorID, TraceID: cmd.TraceID, Now: time.Now().UTC()})
	if errors.Is(runErr, qualityapp.ErrAssessmentAttemptInProgress) || errors.Is(runErr, transaction.ErrAdvisoryLockBusy) {
		return id, runErr
	}
	if runErr == nil && assessment.RuleSetContentSHA256 != c.RecheckRuleHash {
		runErr = domain.ErrEvidence
	}
	if runErr != nil {
		// Preserve PENDING unless the quality attempt owner confirms failure.
		var failed bool
		if err = s.Repo.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quality_assessment_attempt_outcome WHERE attempt_id=$1 AND outcome='FAILED')`, id).Scan(&failed); err != nil {
			return id, err
		}
		if !failed {
			return id, runErr
		}
	}
	// Actual engine failures remain in the existing quality attempt ledger/costs.
	// The case stores a non-secret error category, never a provider message/body.
	err = s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := s.Repo.CaseTx(ctx, tx, cmd.WorkspaceID, cmd.CaseID); e != nil {
			return e
		}
		var outcome string
		var assessmentID *uuid.UUID
		var errorCode string
		if runErr == nil {
			outcome = "ASSESSED"
			assessmentID = &assessment.ID
		} else {
			outcome = "FAILED"
			errorCode = "QUALITY_RECHECK_FAILED"
		}
		tag, e := tx.Exec(ctx, `INSERT INTO quality_remediation_recheck_outcome(id,assessment_id,outcome,error_code) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, id, assessmentID, outcome, errorCode)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return proof(ctx, tx, cmd, c.ID, "QualityRemediationRechecked", map[string]any{"recheckId": id, "assessmentId": assessmentID, "outcome": outcome, "errorCode": errorCode})
	})
	if err != nil {
		return id, err
	}
	return id, runErr
}

func (s *Service) Close(ctx context.Context, cmd Command) (uuid.UUID, error) {
	var id uuid.UUID
	if err := domain.ValidateCommand(cmd.WorkspaceID, cmd.ActorID, cmd.Key); err != nil {
		return id, err
	}
	if err := domain.ValidateReason(cmd.Reason); err != nil {
		return id, err
	}
	err := s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := s.Repo.CaseTx(ctx, tx, cmd.WorkspaceID, cmd.CaseID); e != nil {
			return e
		}
		var actor uuid.UUID
		var key, reason string
		e := tx.QueryRow(ctx, `SELECT id,actor_id,request_key,reason FROM quality_remediation_close WHERE case_id=$1`, cmd.CaseID).Scan(&id, &actor, &key, &reason)
		if e == nil {
			if actor != cmd.ActorID || key != cmd.Key || reason != cmd.Reason {
				return domain.ErrConflict
			}
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, _, e = s.lock(ctx, tx, cmd, true, false); e != nil {
			return e
		}
		id = uuid.New()
		if _, e = tx.Exec(ctx, `INSERT INTO quality_remediation_close(id,case_id,actor_id,request_key,reason) VALUES($1,$2,$3,$4,$5)`, id, cmd.CaseID, cmd.ActorID, cmd.Key, cmd.Reason); e != nil {
			return e
		}
		return proof(ctx, tx, cmd, cmd.CaseID, "QualityRemediationManuallyClosed", map[string]any{"closureId": id, "qualityOrCertificationChanged": false})
	})
	return id, err
}

func proof(ctx context.Context, tx pgx.Tx, cmd Command, id uuid.UUID, eventType string, payload map[string]any) error {
	event, err := outbox.NewEvent("QUALITY_REMEDIATION", id, eventType, payload)
	if err != nil {
		return err
	}
	if err = outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err = audit.Append(ctx, tx, audit.Event{WorkspaceID: &cmd.WorkspaceID, ActorType: "USER", ActorID: &cmd.ActorID, Action: eventType, ObjectType: "QUALITY_REMEDIATION", ObjectID: id, AfterState: payload, Reason: cmd.Reason, TraceID: cmd.TraceID}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: cmd.WorkspaceID, EvidenceType: "QUALITY_REMEDIATION", Title: eventType, SourceType: "QUALITY_REMEDIATION", SourceID: &id, Metadata: payload, CreatedBy: &cmd.ActorID}, evidence.Relation{ObjectType: "QUALITY_REMEDIATION", ObjectID: id, RelationType: "SUPPORTS"})
	return err
}
