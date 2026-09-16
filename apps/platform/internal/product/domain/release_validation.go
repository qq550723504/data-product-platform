package domain

import (
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidReleaseTransition = errors.New("product release state transition is invalid")

func (r *ProductRelease) BeginValidation(contractVersionID, rightsSnapshotID, qualityResultID, complianceResultID uuid.UUID) error {
	if r.Status != ReleaseDraft && r.Status != ReleaseValidating {
		return ErrInvalidReleaseTransition
	}
	if contractVersionID == uuid.Nil || rightsSnapshotID == uuid.Nil || qualityResultID == uuid.Nil || complianceResultID == uuid.Nil {
		return ErrInvalidRelease
	}
	r.ContractVersionID = &contractVersionID
	r.RightsSnapshotID = &rightsSnapshotID
	r.QualityResultID = &qualityResultID
	r.ComplianceResultID = &complianceResultID
	r.Status = ReleaseValidating
	return nil
}

func (r *ProductRelease) MarkReady() error {
	if r.Status != ReleaseValidating {
		return ErrInvalidReleaseTransition
	}
	r.Status = ReleaseReady
	return nil
}
