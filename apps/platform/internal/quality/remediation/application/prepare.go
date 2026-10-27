package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/domain"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type PreparedRepair struct {
	Case           domain.Case
	ReceiptID      uuid.UUID
	ResolutionJobs map[string]uuid.UUID
}

// PrepareAgentRepairTx reads and validates the existing owner's exact bindings.
// The caller already holds its Agent task lock; no Detect/Assign/Execution is
// invoked, no case is mutated, and no cost or business receipt is fabricated.
func (s *Service) PrepareAgentRepairTx(ctx context.Context, tx pgx.Tx, workspace, caseID, workflowID, outputID uuid.UUID, period string, inputs []workflowdomain.InputBinding) (PreparedRepair, error) {
	result := PreparedRepair{ResolutionJobs: map[string]uuid.UUID{}}
	if s == nil || s.Repo == nil || s.Plan.Hash == "" {
		return result, domain.ErrScope
	}
	c, err := s.Repo.CaseTx(ctx, tx, workspace, caseID)
	if err != nil {
		return result, err
	}
	var closed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quality_remediation_close WHERE case_id=$1)`, caseID).Scan(&closed); err != nil {
		return result, err
	}
	if closed {
		return result, domain.ErrClosed
	}
	if c.PlanRef != s.Plan.Ref || c.PlanVersion != s.Plan.Metadata.Version || c.PlanHash != s.Plan.Hash || c.WorkflowHash != s.Plan.WorkflowHash || c.RecheckRuleHash != s.Plan.RecheckHash || strings.TrimSpace(period) != period || !workflowdomain.ValidTargetPeriod(period) {
		return result, domain.ErrScope
	}
	var matches bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_version w JOIN workflow wf ON wf.id=w.workflow_id JOIN dataset d ON d.id=$3 WHERE w.id=$1 AND wf.workspace_id=$2 AND w.definition_sha256=$4 AND d.workspace_id=$2 AND d.code=$5 AND d.dataset_type='CURATED')`, workflowID, workspace, outputID, s.Plan.WorkflowHash, s.Plan.Spec.OutputDatasetCode).Scan(&matches)
	if err != nil {
		return result, err
	}
	if !matches {
		return result, domain.ErrScope
	}
	if err = s.validateInputProvenance(ctx, tx, c, inputs, &result.ReceiptID, result.ResolutionJobs); err != nil {
		return result, err
	}
	// A plan is for an initial repair; a prior execution requires the separately
	// designed RETRY phase and must never be silently recycled as another run.
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quality_remediation_execution WHERE case_id=$1)`, caseID).Scan(&matches); err != nil {
		return result, err
	}
	if matches {
		return result, domain.ErrAttempt
	}
	result.Case = c
	return result, nil
}

func IsPreparationBlocked(err error) bool {
	return errors.Is(err, domain.ErrScope) || errors.Is(err, domain.ErrClosed) || errors.Is(err, domain.ErrAttempt) || errors.Is(err, pgx.ErrNoRows)
}
