package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
)

// CertificationDirectDataGate adapts the HQD-5A Current Delivery Eligibility
// query into the delivery command's fenced gate contract. The caller holds the
// shared delivery authorization fence while this adapter reads the last
// committed certification/rights/dataset facts, so concurrent gate-changing
// commands serialize before or after the ISSUED/BLOCKED linearization point.
type CertificationDirectDataGate struct {
	eligibility *certificationapp.EligibilityService
}

func NewCertificationDirectDataGate(eligibility *certificationapp.EligibilityService) *CertificationDirectDataGate {
	return &CertificationDirectDataGate{eligibility: eligibility}
}

func (g *CertificationDirectDataGate) EvaluateDirectData(ctx context.Context, tx pgx.Tx, request DirectDataGateRequest) (DirectDataGateResult, error) {
	if g == nil || g.eligibility == nil || tx == nil {
		return DirectDataGateResult{}, fmt.Errorf("direct data certification gate is not configured")
	}
	if request.WorkspaceID == uuid.Nil || request.DatasetVersionID == uuid.Nil || request.ProfileID == uuid.Nil {
		return DirectDataGateResult{}, fmt.Errorf("workspace, DatasetVersion, and CertificationProfile are required")
	}
	result, err := g.eligibility.CheckTx(ctx, tx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID: request.WorkspaceID, DatasetVersionID: request.DatasetVersionID, ProfileID: request.ProfileID,
		Consumer: request.EffectiveConsumerRef, Purpose: request.Purpose, Action: request.Action,
		Delivery: request.DeliveryChannel, ScopeType: request.ScopeType, ScopeRef: request.ScopeRef,
		AsOf: time.Now().UTC(),
	})
	if err != nil {
		return DirectDataGateResult{}, err
	}

	blockers := make([]string, 0, len(result.Blockers))
	for _, blocker := range result.Blockers {
		if blocker.Code != "" {
			blockers = append(blockers, blocker.Code)
		}
	}
	evaluation := domain.GateEvaluation{
		Allowed:              result.Allowed,
		Blockers:             blockers,
		PrincipalRef:         request.PrincipalRef,
		EffectiveConsumerRef: request.EffectiveConsumerRef,
		DelegationRef:        "",
	}
	if result.Allowed {
		// DIRECT_DATA has no bearer credential, but the existing DeliveryOperation
		// model requires a finite authorization window. The window is internal to
		// the command and never becomes a client credential lifetime.
		cap := request.RequestedExpiresAt.UTC()
		evaluation.FreshCapExpiresAt = &cap
	}

	var certificationRef *uuid.UUID
	if result.Certification.ID != uuid.Nil {
		id := result.Certification.ID
		certificationRef = &id
	}
	return DirectDataGateResult{Evaluation: evaluation, CertificationRef: certificationRef}, nil
}
