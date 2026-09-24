package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

type ReferenceEvidenceResolver struct{}

func NewReferenceEvidenceResolver() *ReferenceEvidenceResolver {
	return &ReferenceEvidenceResolver{}
}

func (r *ReferenceEvidenceResolver) Resolve(
	_ context.Context,
	cmd EvaluateDatasetCertificationCommand,
	profile domain.ProfileSnapshot,
) (domain.EvaluationInput, error) {
	if cmd.WorkspaceID == uuid.Nil || cmd.DatasetVersionID == uuid.Nil || cmd.QualityAssessmentID == uuid.Nil {
		return domain.EvaluationInput{}, fmt.Errorf("workspace, DatasetVersion, and QualityAssessment references are required")
	}
	input := domain.EvaluationInput{
		WorkspaceID:      cmd.WorkspaceID,
		DatasetVersionID: cmd.DatasetVersionID,
		Quality: domain.QualityAssessmentEvidence{
			ID: cmd.QualityAssessmentID,
		},
		ActorID:  cmd.ActorID,
		IssuedAt: cmd.Now,
	}
	if profile.Rights.Required {
		rights := &domain.RightsEvidence{}
		if cmd.RightsSnapshotID != nil {
			rights.RightsSnapshotID = *cmd.RightsSnapshotID
		}
		if cmd.EffectiveRightsSnapshotID != nil {
			rights.EffectiveRightsSnapshotID = *cmd.EffectiveRightsSnapshotID
		}
		input.Rights = rights
	}
	if profile.ComplianceRequired && cmd.ComplianceResultID != nil {
		input.Compliance = &domain.ComplianceEvidence{ID: *cmd.ComplianceResultID}
	}
	if profile.ContractRequired && cmd.ContractVersionID != nil {
		input.Contract = &domain.ContractEvidence{ID: *cmd.ContractVersionID}
	}
	if profile.TraceabilityRequired && cmd.TraceabilityEvidenceID != nil {
		input.Traceability = &domain.TraceabilityEvidence{ID: *cmd.TraceabilityEvidenceID}
	}
	if profile.EvidenceRequired && cmd.EvidenceSnapshotID != nil {
		input.Evidence = &domain.EvidenceSnapshot{ID: *cmd.EvidenceSnapshotID}
	}
	return input, nil
}
