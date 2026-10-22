package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type historicalRelease struct {
	Product, Version, Release, DatasetVersion, Quality, Compliance, Rights, Evidence uuid.UUID
}

// This guarded synthetic operator reuses real Create/Validate/Publish Commands.
// It cannot insert a PUBLISHED row, change readiness or modify a frozen fact.
// The production worker projects ProductReleased through the existing Adapter.
func prepareHistoricalRelease(ctx context.Context, pool *pgxpool.Pool, tx *transaction.Manager, c config, f facts, label string) {
	path := filepath.Join(c.Private, label+".json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		panic("release fixture requires a new explicit label")
	}
	var h historicalRelease
	rows, err := pool.Query(ctx, `
		SELECT v.id, v.dataset_id, q.id, cr.id
		FROM execution e JOIN dataset_version v ON v.id=e.output_dataset_version_id
		JOIN quality_result q ON q.dataset_version_id=v.id AND q.gate_decision='PASS'
		JOIN compliance_result cr ON cr.dataset_version_id=v.id AND cr.gate_decision='PASS'
		WHERE e.workspace_id=$1 AND e.workflow_version_id=$2 AND e.status='SUCCEEDED' AND v.status='READY'
	`, f.Workspace, f.Workflow)
	must(err)
	var datasetID uuid.UUID
	var count int
	for rows.Next() {
		must(rows.Scan(&h.DatasetVersion, &datasetID, &h.Quality, &h.Compliance))
		count++
	}
	must(rows.Err())
	rows.Close()
	if count != 1 {
		panic("exactly one real browser-produced READY output with PASS quality/compliance required")
	}
	products := productapp.NewService(tx, productinfra.NewPostgresRepository(pool))
	if label == "release-healthy" {
		product, err := products.CreateProduct(ctx, productapp.CreateProductCommand{
			WorkspaceID: f.Workspace, Code: "DP-ENTERPRISE-ACTIVITY", Name: "PGI historical publication",
			Description: "Synthetic #298 publication protection proof", DomainCode: "PARK_ENTERPRISE_ACTIVITY", ActorID: &f.Actor,
		})
		must(err)
		h.Product = product.ID
		version, err := products.CreateVersion(ctx, productapp.CreateVersionCommand{
			ProductID: h.Product, MajorVersion: 1, WorkflowVersionID: &f.Workflow, ContractVersionID: &f.Contract,
			EntityPolicyRef: "park/matching/company-match-policy-v1.yaml", IndicatorSetRef: "park-enterprise-activity@1.0.0",
			Assets:  []productdomain.AssetSpec{{AssetType: productdomain.AssetDataset, Name: "enterprise_activity_curated", DatasetID: &datasetID, DeliveryConfig: map[string]any{"mode": "DATASET", "rawExport": false}}},
			ActorID: &f.Actor,
		})
		must(err)
		h.Version = version.ID
	} else {
		var prior historicalRelease
		read(filepath.Join(c.Private, "release-healthy.json"), &prior)
		h.Product, h.Version = prior.Product, prior.Version
		if label == "release-repaired-outage" {
			if prior.DatasetVersion == h.DatasetVersion {
				panic("repaired publication requires a new browser-produced output")
			}
			var rechecked bool
			must(pool.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM quality_remediation_recheck_outcome o
				JOIN quality_remediation_recheck r ON r.id=o.id
				JOIN quality_remediation_case c ON c.id=r.case_id
				JOIN quality_remediation_execution l ON l.id=r.execution_link_id
				JOIN execution e ON e.id=l.execution_id
				WHERE c.workspace_id=$1 AND e.output_dataset_version_id=$2
				AND o.outcome='ASSESSED' AND o.assessment_id=$3
			)`, f.Workspace, h.DatasetVersion, h.Quality).Scan(&rechecked))
			if !rechecked {
				panic("repaired publication requires the actual C1 output recheck")
			}
			version, err := products.CreateVersion(ctx, productapp.CreateVersionCommand{
				ProductID: h.Product, MajorVersion: 2, WorkflowVersionID: &f.Workflow, ContractVersionID: &f.Contract,
				EntityPolicyRef: "park/matching/company-match-policy-v1.yaml", IndicatorSetRef: "park-enterprise-activity@1.0.0",
				Assets:  []productdomain.AssetSpec{{AssetType: productdomain.AssetDataset, Name: "enterprise_activity_curated", DatasetID: &datasetID, DeliveryConfig: map[string]any{"mode": "DATASET", "rawExport": false}}},
				ActorID: &f.Actor,
			})
			must(err)
			h.Version = version.ID
		} else if prior.DatasetVersion != h.DatasetVersion {
			panic("publication retry must use the same immutable browser output")
		}
	}
	release, err := products.CreateRelease(ctx, productapp.CreateReleaseCommand{
		ProductID: h.Product, ProductVersionID: h.Version, ReleaseNo: label,
		Datasets:     []productdomain.ReleaseDataset{{DatasetVersionID: h.DatasetVersion, Role: productdomain.DatasetPrimary}},
		ReleaseNotes: "Synthetic provider outage protection proof", ActorID: &f.Actor,
	})
	must(err)
	h.Release = release.ID
	rights := rightsapp.NewService(tx, rightsinfra.NewPostgresRepository(pool))
	snapshot, err := rights.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID: f.Workspace, ProductReleaseID: &release.ID, Purpose: purpose, ConsumerRef: "LICENSED_BANK",
		AsOf: time.Now().UTC(), AuthorizationIDs: []uuid.UUID{f.Authorization}, ActorID: &f.Actor,
	})
	must(err)
	h.Rights = snapshot.ID
	readiness, err := products.ValidateRelease(ctx, productapp.ValidateReleaseCommand{
		ReleaseID: release.ID, ContractVersionID: f.Contract, RightsSnapshotID: h.Rights,
		QualityResultID: h.Quality, ComplianceResultID: h.Compliance, ActorID: &f.Actor,
	})
	must(err)
	if readiness.Overall != "READY" || len(readiness.Blockers) != 0 {
		panic(fmt.Sprintf("real release readiness denied: %+v", readiness))
	}
	published, err := products.PublishRelease(ctx, productapp.PublishReleaseCommand{
		ReleaseID: release.ID, IdempotencyKey: "pgi-history-" + f.Workspace.String() + "-" + label, ActorID: &f.Actor,
	})
	must(err)
	if published.Status != productdomain.ReleasePublished || published.EvidenceSnapshotID == nil {
		panic("real PublishRelease did not freeze a published evidence snapshot")
	}
	h.Evidence = *published.EvidenceSnapshotID
	write(path, h)
	fmt.Printf("real published release %s; evidence snapshot %s\n", h.Release, h.Evidence)
}
