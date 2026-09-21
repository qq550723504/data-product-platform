package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type CertificationHistoryItem struct {
	Certification certificationdomain.DatasetCertification
	Dispositions  []certificationdomain.CertificationDisposition
}

func (s *CertificationService) ListDatasetHistory(ctx context.Context, workspaceID, datasetVersionID uuid.UUID, asOf time.Time) ([]CertificationHistoryItem, error) {
	if workspaceID == uuid.Nil || datasetVersionID == uuid.Nil {
		return nil, fmt.Errorf("workspace and DatasetVersion are required")
	}
	if s == nil || s.profileRepo == nil || s.certificationRepo == nil {
		return nil, fmt.Errorf("certification query dependencies are incomplete")
	}
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	profileIDs, err := s.certificationRepo.ListProfileIDsForDatasetVersion(ctx, workspaceID, datasetVersionID, asOf)
	if err != nil {
		return nil, err
	}
	items := make([]CertificationHistoryItem, 0)
	for _, profileID := range profileIDs {
		profile, err := s.profileRepo.GetProfile(ctx, profileID)
		if err != nil {
			return nil, err
		}
		if profile.WorkspaceID != workspaceID {
			return nil, fmt.Errorf("certification profile crosses workspace boundary")
		}
		history, err := s.certificationRepo.ListCertificationHistory(ctx, workspaceID, datasetVersionID, profileID, asOf, profile)
		if err != nil {
			return nil, err
		}
		for _, certification := range history.Certifications {
			item := CertificationHistoryItem{Certification: certification}
			for _, disposition := range history.Dispositions {
				if disposition.CertificationID == certification.ID {
					item.Dispositions = append(item.Dispositions, disposition)
				}
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Certification.IssuedAt.Equal(items[j].Certification.IssuedAt) {
			return items[i].Certification.ID.String() > items[j].Certification.ID.String()
		}
		return items[i].Certification.IssuedAt.After(items[j].Certification.IssuedAt)
	})
	return items, nil
}

type DeliveryEligibilityQuery struct {
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	ProfileID        uuid.UUID
	Consumer         string
	Purpose          string
	Action           string
	Delivery         string
	AsOf             time.Time
}

type EntitlementCheck struct {
	DataResourceID uuid.UUID
	Path           rightsdomain.EntitlementPath
	Decision       rightsdomain.EntitlementDecision
}

type DeliveryEligibilityResult struct {
	Allowed              bool
	DatasetVersionStatus datasetdomain.VersionStatus
	DatasetVersionGate   certificationdomain.GateResult
	Certification        certificationdomain.DatasetCertification
	CertificationGate    certificationdomain.GateResult
	EntitlementGate      certificationdomain.GateResult
	EntitlementChecks    []EntitlementCheck
	Blockers             []certificationdomain.Blocker
}

type EligibilityService struct {
	certifications *CertificationService
	datasets       *datasetinfra.PostgresRepository
	rights         *rightsinfra.PostgresRepository
}

func NewEligibilityService(certifications *CertificationService, datasets *datasetinfra.PostgresRepository, rights *rightsinfra.PostgresRepository) *EligibilityService {
	return &EligibilityService{certifications: certifications, datasets: datasets, rights: rights}
}

func (s *EligibilityService) Check(ctx context.Context, query DeliveryEligibilityQuery) (DeliveryEligibilityResult, error) {
	result := DeliveryEligibilityResult{
		DatasetVersionGate: certificationdomain.GateResult{Blockers: []certificationdomain.Blocker{}},
		CertificationGate:  certificationdomain.GateResult{Blockers: []certificationdomain.Blocker{}},
		EntitlementGate:    certificationdomain.GateResult{Blockers: []certificationdomain.Blocker{}},
		Blockers:           []certificationdomain.Blocker{},
	}
	if s == nil || s.certifications == nil || s.datasets == nil || s.rights == nil {
		return result, fmt.Errorf("delivery eligibility dependencies are incomplete")
	}
	if query.WorkspaceID == uuid.Nil || query.DatasetVersionID == uuid.Nil || query.ProfileID == uuid.Nil {
		return result, fmt.Errorf("workspace, DatasetVersion, and profile are required")
	}
	query.Consumer = strings.TrimSpace(query.Consumer)
	query.Purpose = strings.ToUpper(strings.TrimSpace(query.Purpose))
	query.Action = strings.ToUpper(strings.TrimSpace(query.Action))
	query.Delivery = strings.ToUpper(strings.TrimSpace(query.Delivery))
	if query.Consumer == "" || query.Purpose == "" || query.Action == "" || query.Delivery == "" {
		return result, fmt.Errorf("consumer, purpose, action, and delivery are required")
	}
	if query.AsOf.IsZero() {
		query.AsOf = time.Now().UTC()
	}

	workspaceID, err := s.rights.DatasetVersionWorkspace(ctx, query.DatasetVersionID)
	if err != nil {
		return result, err
	}
	if workspaceID != query.WorkspaceID {
		return result, fmt.Errorf("DatasetVersion crosses workspace boundary")
	}
	version, err := s.datasets.GetVersion(ctx, query.DatasetVersionID)
	if err != nil {
		return result, err
	}
	result.DatasetVersionStatus = version.Status
	switch version.Status {
	case datasetdomain.VersionReady, datasetdomain.VersionSuperseded:
		result.DatasetVersionGate.Allowed = true
	case datasetdomain.VersionInvalid:
		result.DatasetVersionGate.Blockers = append(result.DatasetVersionGate.Blockers, certificationdomain.Blocker{Code: "DATASET_VERSION_INVALID", Detail: "DatasetVersion is INVALID"})
	default:
		result.DatasetVersionGate.Blockers = append(result.DatasetVersionGate.Blockers, certificationdomain.Blocker{Code: "DATASET_VERSION_NOT_DELIVERABLE", Detail: "DatasetVersion must be READY or an explicitly requested SUPERSEDED historical version"})
	}
	result.Blockers = append(result.Blockers, result.DatasetVersionGate.Blockers...)

	current, err := s.certifications.CheckCurrent(ctx, CurrentCertificationQuery{
		WorkspaceID: query.WorkspaceID, DatasetVersionID: query.DatasetVersionID, ProfileID: query.ProfileID, AsOf: query.AsOf,
		DeliveryContext: certificationdomain.DeliveryContext{Purpose: query.Purpose, Action: query.Action, Consumer: query.Consumer, Delivery: query.Delivery},
	})
	if err != nil {
		return result, err
	}
	result.Certification = current.Certification
	result.CertificationGate = current.Gate
	result.Blockers = append(result.Blockers, current.Gate.Blockers...)

	if current.Certification.ID != uuid.Nil && current.Gate.Allowed {
		currentInputs, err := s.rights.RequiredLineageInputs(ctx, query.DatasetVersionID)
		if err != nil {
			return result, err
		}
		if len(currentInputs) == 0 {
			result.EntitlementGate.Blockers = append(result.EntitlementGate.Blockers, certificationdomain.Blocker{Code: "CURRENT_ENTITLEMENT_INPUTS_MISSING", Detail: "DatasetVersion has no mapped required source inputs to revalidate"})
		} else {
			if current.Certification.EffectiveRightsSnapshotID != nil {
				snapshot, err := s.rights.GetEffectiveRights(ctx, *current.Certification.EffectiveRightsSnapshotID)
				if err != nil {
					return result, err
				}
				if snapshot.WorkspaceID != query.WorkspaceID || snapshot.TargetDatasetVersionID != query.DatasetVersionID || snapshot.Status != "FINALIZED" {
					result.EntitlementGate.Blockers = append(result.EntitlementGate.Blockers, certificationdomain.Blocker{Code: "CURRENT_ENTITLEMENT_EVIDENCE_INVALID", Detail: "EffectiveRightsSnapshot does not match the requested DatasetVersion"})
				} else if !sameEligibilityLineage(snapshot.Inputs, currentInputs) {
					result.EntitlementGate.Blockers = append(result.EntitlementGate.Blockers, certificationdomain.Blocker{Code: "CURRENT_ENTITLEMENT_LINEAGE_MISMATCH", Detail: "current DatasetVersion required lineage no longer matches the frozen EffectiveRights input membership"})
				}
			}
			if len(result.EntitlementGate.Blockers) == 0 {
				for _, input := range currentInputs {
					check, blocker, err := s.checkInputEntitlement(ctx, query, input)
					if err != nil {
						return result, err
					}
					result.EntitlementChecks = append(result.EntitlementChecks, check)
					if blocker != nil {
						result.EntitlementGate.Blockers = append(result.EntitlementGate.Blockers, *blocker)
					}
				}
			}
		}
	} else {
		result.EntitlementGate.Blockers = append(result.EntitlementGate.Blockers, certificationdomain.Blocker{Code: "CURRENT_ENTITLEMENT_NOT_EVALUATED", Detail: "current entitlement is not evaluated until a current certification covers the requested context"})
	}
	result.EntitlementGate.Allowed = len(result.EntitlementGate.Blockers) == 0
	result.Blockers = append(result.Blockers, result.EntitlementGate.Blockers...)
	result.Allowed = result.DatasetVersionGate.Allowed && result.CertificationGate.Allowed && result.EntitlementGate.Allowed
	return result, nil
}

func sameEligibilityLineage(frozen []rightsdomain.EffectiveRightsInput, current []rightsinfra.LineageInput) bool {
	if len(frozen) != len(current) {
		return false
	}
	frozenIDs := make([]string, 0, len(frozen))
	currentIDs := make([]string, 0, len(current))
	for _, input := range frozen {
		frozenIDs = append(frozenIDs, input.InputDatasetVersionID.String())
	}
	for _, input := range current {
		if !input.ResourceMapped {
			return false
		}
		currentIDs = append(currentIDs, input.DatasetVersionID.String())
	}
	sort.Strings(frozenIDs)
	sort.Strings(currentIDs)
	for i := range frozenIDs {
		if frozenIDs[i] != currentIDs[i] {
			return false
		}
	}
	return true
}

func (s *EligibilityService) checkInputEntitlement(ctx context.Context, query DeliveryEligibilityQuery, input rightsinfra.LineageInput) (EntitlementCheck, *certificationdomain.Blocker, error) {
	check := EntitlementCheck{DataResourceID: input.DataResourceID, Path: rightsdomain.EntitlementDirectUse}
	if !input.ResourceMapped || input.DataResourceID == uuid.Nil {
		return check, &certificationdomain.Blocker{Code: "CURRENT_ENTITLEMENT_RESOURCE_UNMAPPED", Detail: input.DatasetVersionID.String() + ": required source DatasetVersion has no DataResource mapping"}, nil
	}
	request := rightsdomain.EntitlementRequest{
		WorkspaceID: query.WorkspaceID, DataResourceID: input.DataResourceID, ConsumerRef: query.Consumer,
		Purpose: query.Purpose, Action: query.Action, AsOf: query.AsOf,
		Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: input.DataResourceID.String()},
		Path:  rightsdomain.EntitlementDirectUse,
	}
	direct, err := s.rights.CheckCurrentEntitlement(ctx, request)
	if err != nil {
		return check, nil, err
	}
	check.Decision = direct
	if direct.Decision == rightsdomain.DecisionAllowed {
		return check, nil, nil
	}

	request.Path = rightsdomain.EntitlementDownstream
	check.Path = request.Path
	downstream, err := s.rights.CheckCurrentEntitlement(ctx, request)
	if err != nil {
		return check, nil, err
	}
	check.Decision = downstream
	if downstream.Decision == rightsdomain.DecisionAllowed {
		return check, nil, nil
	}
	return check, &certificationdomain.Blocker{
		Code:   "CURRENT_ENTITLEMENT_BLOCKED",
		Detail: input.DataResourceID.String() + ": direct use blocked (" + direct.Reason + "); downstream authorization blocked (" + downstream.Reason + ")",
	}, nil
}
