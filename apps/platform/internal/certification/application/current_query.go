package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

type CurrentCertificationQuery struct {
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	ProfileID        uuid.UUID
	AsOf             time.Time
	DeliveryContext  domain.DeliveryContext
}

type CurrentCertificationResult struct {
	Certification domain.DatasetCertification
	Gate          domain.GateResult
}

// CheckCurrent resolves current certification from append-only history. It
// never treats the newest row as current and returns a blocked result when no
// current certification exists. Ambiguous history is an error so callers
// cannot accidentally turn two current facts into an authorization.
func (s *CertificationService) CheckCurrent(ctx context.Context, query CurrentCertificationQuery) (CurrentCertificationResult, error) {
	if query.WorkspaceID == uuid.Nil || query.DatasetVersionID == uuid.Nil || query.ProfileID == uuid.Nil {
		return CurrentCertificationResult{}, fmt.Errorf("workspace, DatasetVersion, and profile are required")
	}
	if s == nil || s.profileRepo == nil || s.certificationRepo == nil {
		return CurrentCertificationResult{}, fmt.Errorf("certification query dependencies are incomplete")
	}
	asOf := query.AsOf
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	profile, err := s.profileRepo.GetProfile(ctx, query.ProfileID)
	if err != nil {
		return CurrentCertificationResult{}, err
	}
	if profile.WorkspaceID != query.WorkspaceID {
		return CurrentCertificationResult{}, fmt.Errorf("certification profile does not belong to the query workspace")
	}
	history, err := s.certificationRepo.ListCertificationHistory(ctx, query.WorkspaceID, query.DatasetVersionID, query.ProfileID, asOf, profile)
	if err != nil {
		return CurrentCertificationResult{}, err
	}
	certification, err := domain.SelectCurrent(history.Certifications, history.Dispositions, query.WorkspaceID, query.DatasetVersionID, profile.ProfileRef, asOf)
	if err != nil {
		return CurrentCertificationResult{}, err
	}
	if certification.ID == uuid.Nil {
		return CurrentCertificationResult{
			Gate: domain.GateResult{
				Allowed:  false,
				Blockers: []domain.Blocker{{Code: "CERTIFICATION_NOT_CURRENT", Detail: "no current CERTIFIED fact matches the requested workspace, DatasetVersion, profile, and as_of"}},
			},
		}, nil
	}
	return CurrentCertificationResult{
		Certification: certification,
		Gate:          certification.CheckCurrent(asOf, history.Dispositions, query.DeliveryContext),
	}, nil
}
