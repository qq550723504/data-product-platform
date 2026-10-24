package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type remainingState struct {
	Plane                            planeState
	P0, P1, P2, T1Profile, T2Profile uuid.UUID
}

func remainingWriter(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, o *lc.OccupancyService, pins *lc.FixtureSubjectPins, source domain.Copy, state planeState) (*datasetapp.UploadVersionService, error) {
	c := r.Base
	var ns uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[0]).Scan(&ns); err != nil {
		return nil, err
	}
	slots := []lc.ProcessingOutputSlot{{DatasetID: state.StandardizedDatasetID, NamespaceID: ns, Namespace: namespaces[0], Key: "fixture/remaining-standardized.csv"}, {DatasetID: state.OutputDatasetID, NamespaceID: ns, Namespace: namespaces[0], Key: "fixture/remaining-processed.csv"}}
	j := lc.FixtureOutputJournal{Service: pins, Slots: slots, Observations: lc.NewPostgresStorageCallJournal(o)}
	writer, err := storage.NewLifecycleOutputWriter(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.ProducerAccess, SecretKey: c.ProducerSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: j})
	if err != nil {
		return nil, err
	}
	store := &planeOutput{slots: map[uuid.UUID]lc.ProcessingOutputSlot{slots[0].DatasetID: slots[0], slots[1].DatasetID: slots[1]}, writer: writer, pins: pins, source: source}
	return datasetapp.NewUploadVersionService(transaction.NewManager(pool), datasetinfra.NewPostgresRepository(pool), store).WithPublicationLease(j), nil
}
func remainingSetup(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy) error {
	if r.Group != "business-races" {
		return lc.ErrStorageScope
	}
	c := r.Base
	var s remainingState
	tx := transaction.NewManager(pool)
	datasets := datasetinfra.NewPostgresRepository(pool)
	create := datasetapp.NewCreateDatasetService(tx, datasets, resourceinfra.NewPostgresRepository())
	// A finite continuation of the retained e311423a setup failure. That helper
	// committed exactly these two empty datasets before its policy constructor
	// rejected the composition. No prior business fact is replaced or retried.
	resume := false
	resumePath := filepath.Join("/run/private", "setup-resume.json")
	if _, err := os.Stat(resumePath); err == nil {
		var prior planeState
		if err = readRemaining(resumePath, &prior); err != nil {
			return err
		}
		if prior.StandardizedDatasetID != uuid.MustParse("667e9747-242e-418b-aa37-e1ea46d92aca") || prior.OutputDatasetID != uuid.MustParse("7b5f045a-170d-4f10-b7c2-2d5d609940d5") || prior.StandardizedVersionID != uuid.Nil || prior.WorkflowVersionID != uuid.Nil || prior.ProfileID != uuid.Nil {
			return lc.ErrStorageScope
		}
		s.Plane = prior
		resume = true
	}
	for i, kind := range []datasetdomain.DatasetType{datasetdomain.DatasetTypeStandardized, datasetdomain.DatasetTypeCurated} {
		code := "REMAINING-STANDARDIZED"
		if i == 1 {
			code = "REMAINING-CURATED"
		}
		if resume {
			id := s.Plane.StandardizedDatasetID
			if i == 1 {
				id = s.Plane.OutputDatasetID
			}
			var valid bool
			if err := pool.QueryRow(ctx, `SELECT workspace_id=$2 AND code=$3 AND dataset_type=$4 AND created_by=$5 AND NOT EXISTS(SELECT 1 FROM dataset_version WHERE dataset_id=d.id) FROM dataset d WHERE id=$1`, id, c.WorkspaceID, code, string(kind), c.ActorID).Scan(&valid); err != nil || !valid {
				return errors.New("partial setup identities changed")
			}
			continue
		}
		d, err := create.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: c.WorkspaceID, Code: code, Name: "Remaining D1 synthetic output", DatasetType: kind, ActorID: &c.ActorID})
		if err != nil {
			return err
		}
		if i == 0 {
			s.Plane.StandardizedDatasetID = d.ID
		} else {
			s.Plane.OutputDatasetID = d.ID
		}
	}
	o, policy, _, pins, err := remainingServices(r, pool, cs)
	if err != nil {
		return err
	}
	for _, cp := range cs[:4] {
		if _, err = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "remaining-policy:" + cp.ID.String()}, DatasetVersionID: cp.DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt}); err != nil {
			return err
		}
	}
	writer, err := remainingWriter(ctx, r, pool, o, pins, cs[0], s.Plane)
	if err != nil {
		return err
	}
	cache := planeCache{}
	payload := pilot.PlaneSyntheticPayloads()
	for i, cp := range cs[:4] {
		b := payload[0]
		if i == 2 {
			b = payload[1]
		}
		if i == 3 {
			b = []byte("id,value\n1,synthetic-remaining-business-races-t2\n")
		}
		cache["s3://"+cp.Namespace+"/"+cp.Key] = b
	}
	match := entityapp.NewMatchService("/run/repo/industry-packs", tx, entityinfra.NewPostgresRepository(pool), datasets, writer, cache)
	job, err := match.Start(ctx, entityapp.StartJobCommand{WorkspaceID: c.WorkspaceID, InputDatasetVersionID: cs[0].DatasetVersionID, OutputDatasetID: s.Plane.StandardizedDatasetID, SourceType: "CSV", SourceRef: "p1.csv", SourceRole: entitydomain.SourceAnchor, PolicyRef: "park/matching/company-match-policy-v1.yaml", ActorID: &c.ActorID})
	if err != nil {
		return err
	}
	if job.Status != entitydomain.JobSucceeded || job.OutputDatasetVersionID == nil {
		return errors.New("remaining anchor resolution failed")
	}
	s.Plane.StandardizedVersionID = *job.OutputDatasetVersionID
	definition := []byte("apiVersion: dataprod.platform/v1alpha1\nkind: Workflow\nmetadata:\n  name: remaining-synthetic-native\n  version: 1.0.0\nspec:\n  processor: enterprise-activity-native-v1\n")
	wf := workflowinfra.NewPostgresRepository(pool)
	version, err := workflowapp.NewWorkflowVersionService(tx, wf).Create(ctx, workflowapp.CreateWorkflowVersionCommand{WorkspaceID: c.WorkspaceID, Code: "REMAINING-NATIVE", Name: "Remaining native processing", Version: "1", DefinitionRef: "remaining-synthetic-native", DefinitionYAML: definition, ActorID: &c.ActorID})
	if err != nil {
		return err
	}
	s.Plane.WorkflowVersionID = version.ID
	svc := workflowapp.NewExecutionService(tx, wf)
	for i, cp := range []domain.Copy{cs[0], cs[0], cs[1]} {
		execution, err := svc.Create(ctx, workflowapp.CreateExecutionCommand{WorkspaceID: c.WorkspaceID, WorkflowVersionID: version.ID, OutputDatasetID: s.Plane.OutputDatasetID, TargetPeriod: pilot.PlanePeriod, Inputs: []workflowdomain.InputBinding{{Name: "enterprise_raw", DatasetVersionID: cp.DatasetVersionID}, {Name: "enterprise_resolution", DatasetVersionID: s.Plane.StandardizedVersionID}, {Name: "lease_raw", DatasetVersionID: cs[2].DatasetVersionID}, {Name: "energy_raw", DatasetVersionID: cs[2].DatasetVersionID}}, ActorID: &c.ActorID, IdempotencyKey: codeForRemainingExecution(i)})
		if err != nil {
			return err
		}
		if i == 0 {
			s.P0 = execution.ID
		} else if i == 1 {
			s.P1 = execution.ID
		} else {
			s.P2 = execution.ID
		}
	}
	s.T1Profile, err = remainingCertification(ctx, c, pool, []domain.Copy{cs[0], cs[2]}, cache, "T1")
	if err != nil {
		return err
	}
	s.T2Profile, err = remainingCertification(ctx, c, pool, []domain.Copy{cs[1], cs[3]}, cache, "T2")
	if err != nil {
		return err
	}
	return write(c, "remaining-state.json", s)
}
func codeForRemainingExecution(i int) string {
	return []string{"remaining-baseline", "remaining-P1", "remaining-P2"}[i]
}
func remainingFreezePolicy(ctx context.Context, r remainingConfig, pool *pgxpool.Pool) error {
	until, e := time.Parse(time.RFC3339, pilot.RemainingNotAfter)
	if e != nil || r.Group != "business-races" || !r.CandidateUseUntil.Equal(until) || len(r.ExpectedPolicyHeads) != 6 {
		return lc.ErrStorageScope
	}
	// Preserve the Core command's one-hour policy cap. The policy command runs
	// afresh inside the confirmed window before any business owner starts.
	policyUntil := until
	if r.Base.ExpiresAt.Before(policyUntil) {
		policyUntil = r.Base.ExpiresAt
	}
	all, e := planeCopies(ctx, r.Base, pool)
	if e != nil {
		return e
	}
	if e = write(r.Base, "candidate07-policy-started.json", map[string]any{"at": time.Now().UTC(), "use_until": until, "one_shot": true}); e != nil {
		return e
	}
	o, policy, _, _, e := remainingServices(r, pool, all)
	_ = o
	if e != nil {
		return e
	}
	var facts []lc.FactResult
	for _, cp := range all {
		if cp.Key == "fixture/control-hold.csv" || cp.Key == "fixture/control-retention.csv" {
			continue
		}
		var previous uuid.UUID
		if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1 AND dataset_version_id=$2`, r.Base.WorkspaceID, cp.DatasetVersionID).Scan(&previous); e != nil {
			return e
		}
		if previous != r.ExpectedPolicyHeads[cp.DatasetVersionID.String()] {
			return errors.New("frozen expected policy head drifted")
		}
		f, e := policy.RenewFixtureCopyPolicy(ctx, lc.RenewFixtureCopyPolicyCommand{PreviousPolicyID: previous, CommandHeader: lc.CommandHeader{WorkspaceID: r.Base.WorkspaceID, IdempotencyKey: "candidate07-policy:" + r.Batch.String() + ":" + cp.ID.String()}, DatasetVersionID: cp.DatasetVersionID, UseUntil: policyUntil, RestoreUntil: policyUntil})
		if e != nil {
			return e
		}
		facts = append(facts, f)
	}
	if len(facts) != 6 {
		return lc.ErrStorageScope
	}
	scope, _ := pilot.RemainingScopeFor(r.Group)
	rightsConfig := r.Base
	rightsConfig.ExpiresAt = until
	if e = remainingRights(ctx, rightsConfig, pool, scope.Copies[:4]); e != nil {
		return e
	}
	return write(r.Base, "candidate07-copy-policies.json", map[string]any{"facts": facts, "use_until": policyUntil, "restore_until": policyUntil, "candidate_window_end": until, "refresh_inside_confirmed_window_required": policyUntil.Before(until), "native_io": 0, "delete": 0})
}
