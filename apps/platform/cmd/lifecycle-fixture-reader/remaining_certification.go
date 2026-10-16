package main

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

// Existing native Quality and Certification Commands produce actual immutable
// facts. Bootstrap cache contains the exact synthetic bytes already verified by
// preparation against MinIO; it has no native read credentials or fallback.
// This fixture profile measures basic CSV completeness, not production readiness.
// Delivery still independently runs the real current entitlement gate.
func remainingCertification(ctx context.Context, c config, pool *pgxpool.Pool, cs []domain.Copy, cache planeCache, caseName string) (uuid.UUID, error) {
	tx := transaction.NewManager(pool)
	datasets := datasetinfra.NewPostgresRepository(pool)
	if err := remainingRights(ctx, c, pool, cs[:2]); err != nil {
		return uuid.Nil, err
	}
	quality, e := qualityapp.NewService("/run/rules", tx, datasets, qualityinfra.NewPostgresRepository(pool), cache).Run(ctx, qualityapp.RunCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: cs[1].DatasetVersionID, RuleSetRef: "fixture/remaining-" + caseName + ".yaml", AssessmentAttemptID: uuid.New(), ActorID: &c.ActorID})
	if e != nil {
		return uuid.Nil, e
	}
	if string(quality.GateDecision) != "PASS" {
		return uuid.Nil, errors.New("synthetic quality gate did not pass")
	}
	profileRepo := certificationinfra.NewProfileRepository(pool)
	profile, e := certificationapp.NewProfileService(tx, profileRepo).Create(ctx, certificationapp.CreateProfileCommand{WorkspaceID: c.WorkspaceID, ActorID: &c.ActorID, Profile: certificationdomain.CertificationProfile{ProfileRef: "fixture/remaining-" + caseName, Code: "REMAINING-" + caseName, Name: "Synthetic CSV completeness", Version: "1", Purpose: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{pilot.PlanePurpose}}, Actions: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}}, Consumers: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{pilot.PlaneConsumer}}, Delivery: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"DIRECT_DATA"}}, QualityGateRequired: true, RequiredCriticalRules: []string{"LC-SYNTHETIC-ID"}}})
	if e != nil {
		return uuid.Nil, e
	}
	service := certificationapp.NewCertificationService(tx, profileRepo, certificationinfra.NewCertificationRepository(pool), certificationapp.NewReferenceEvidenceResolver())
	cert, e := service.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{WorkspaceID: c.WorkspaceID, DatasetVersionID: cs[1].DatasetVersionID, ProfileID: profile.ID, QualityAssessmentID: quality.ID, IdempotencyKey: "remaining-certification:" + caseName, ActorID: &c.ActorID})
	if e != nil {
		return uuid.Nil, e
	}
	if string(cert.Decision) != "CERTIFIED" {
		return uuid.Nil, errors.New("synthetic certification did not certify")
	}
	if e = write(c, "remaining-certification-"+caseName+".json", map[string]any{"quality_id": quality.ID, "certification_id": cert.ID, "profile_id": profile.ID, "profile_scope": "synthetic CSV completeness only; current entitlement independently required", "rights_declarations": "VERIFIED controlled synthetic use only"}); e != nil {
		return uuid.Nil, e
	}
	return profile.ID, nil
}

func remainingRights(ctx context.Context, c config, pool *pgxpool.Pool, cs []domain.Copy) error {
	tx := transaction.NewManager(pool)
	rights := rightsapp.NewService(tx, rightsinfra.NewPostgresRepository(pool))
	for _, cp := range cs {
		var resource uuid.UUID
		if e := pool.QueryRow(ctx, `SELECT d.source_resource_id FROM dataset d JOIN dataset_version v ON v.dataset_id=d.id WHERE v.id=$1`, cp.DatasetVersionID).Scan(&resource); e != nil {
			return e
		}
		from, to := c.StartedAt, c.ExpiresAt
		permissions := []rightsdomain.RightsPermission{}
		for _, action := range []string{"READ", "PROCESS", "AGGREGATE", "DERIVE"} {
			permissions = append(permissions, rightsdomain.RightsPermission{Kind: rightsdomain.PermissionUse, Action: action, Purpose: pilot.PlanePurpose, Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resource.String()}})
		}
		d, e := rights.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{Spec: rightsdomain.RightsDeclarationSpec{WorkspaceID: c.WorkspaceID, DataResourceID: resource, ClaimantRef: pilot.PlaneConsumer, BasisType: "LICENSE", BasisRef: "CONTROLLED_SYNTHETIC_FIXTURE_ONLY", ConsumerScopeType: "EXPLICIT", ConsumerRef: pilot.PlaneConsumer, EffectiveFrom: &from, EffectiveTo: &to, Parties: []rightsdomain.RightsParty{{PartyRef: pilot.PlaneConsumer, Role: "RIGHTS_HOLDER"}}, Permissions: permissions, ActorID: &c.ActorID}})
		if e != nil {
			return e
		}
		if _, e = rights.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{DeclarationID: d.ID, Outcome: rightsdomain.DeclarationVerified, Reason: "Operator-owned synthetic input only", ActorID: &c.ActorID}); e != nil {
			return e
		}
	}
	return nil
}
