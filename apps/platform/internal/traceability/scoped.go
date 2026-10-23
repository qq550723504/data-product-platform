package traceability

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
)

var ErrScope = errors.New("trace workspace boundary cannot be confirmed")

// ObjectWorkspace uses fixed platform ownership relations, never caller supplied SQL.
func (r *Repository) ObjectWorkspace(ctx context.Context, kind string, id uuid.UUID) (uuid.UUID, error) {
	var query string
	switch kind {
	case "PRODUCT_RELEASE":
		query = `SELECT p.workspace_id FROM product_release r JOIN data_product p ON p.id=r.product_id WHERE r.id=$1`
	case "EXECUTION":
		query = `SELECT workspace_id FROM execution WHERE id=$1`
	default:
		return uuid.Nil, ErrScope
	}
	var ws uuid.UUID
	err := r.query.QueryRow(ctx, query, id).Scan(&ws)
	return ws, err
}

// Every returned historical member must have independently verifiable ownership.
// A corrupt cross-workspace edge fails the entire read rather than disappearing.
func (r *Repository) ProductReleaseInWorkspace(ctx context.Context, ws, id uuid.UUID) (ReleaseTrace, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return ReleaseTrace{}, err
	}
	defer tx.Rollback(ctx)
	reader := &Repository{query: tx, evidenceRepo: evidence.NewQueryRepository(tx), costRepo: cost.NewQueryRepository(tx)}
	result, err := reader.productReleaseInWorkspace(ctx, ws, id)
	if err != nil {
		return ReleaseTrace{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReleaseTrace{}, err
	}
	return result, nil
}
func (r *Repository) productReleaseInWorkspace(ctx context.Context, ws, id uuid.UUID) (ReleaseTrace, error) {
	owner, err := r.ObjectWorkspace(ctx, "PRODUCT_RELEASE", id)
	if err != nil {
		return ReleaseTrace{}, err
	}
	if owner != ws {
		return ReleaseTrace{}, pgx.ErrNoRows
	}
	t, err := r.ProductRelease(ctx, id)
	if err != nil {
		return ReleaseTrace{}, err
	}
	check := func(query string, id uuid.UUID) error {
		var owner uuid.UUID
		err := r.query.QueryRow(ctx, query, id).Scan(&owner)
		if err != nil || owner != ws {
			return ErrScope
		}
		return nil
	}
	if err = check(`SELECT p.workspace_id FROM product_version v JOIN data_product p ON p.id=v.product_id WHERE v.id=$1`, t.ProductVersionID); err != nil {
		return ReleaseTrace{}, err
	}
	if t.EvidenceSnapshot != nil {
		if err = check(`SELECT workspace_id FROM evidence_snapshot WHERE id=$1`, t.EvidenceSnapshot.ID); err != nil {
			return ReleaseTrace{}, err
		}
	}
	for _, v := range t.DatasetVersions {
		if err = check(`SELECT d.workspace_id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=$1`, v.ID); err != nil {
			return ReleaseTrace{}, err
		}
	}
	for _, v := range t.Executions {
		if err = check(`SELECT workspace_id FROM execution WHERE id=$1`, v.ID); err != nil {
			return ReleaseTrace{}, err
		}
		if err = check(`SELECT w.workspace_id FROM workflow_version v JOIN workflow w ON w.id=v.workflow_id WHERE v.id=$1`, v.WorkflowVersionID); err != nil {
			return ReleaseTrace{}, err
		}
		for _, d := range v.DependencyBindings {
			if d.DatasetVersionID != nil {
				if err = check(`SELECT d.workspace_id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.id=$1`, *d.DatasetVersionID); err != nil {
					return ReleaseTrace{}, err
				}
			}
		}
	}
	for _, v := range t.EntityMatchJobs {
		if v.WorkspaceID != ws {
			return ReleaseTrace{}, ErrScope
		}
	}
	for _, v := range t.EntityMappings {
		if err = check(`SELECT workspace_id FROM entity_mapping WHERE id=$1`, v.ID); err != nil {
			return ReleaseTrace{}, err
		}
		if err = check(`SELECT workspace_id FROM entity WHERE id=$1`, v.EntityID); err != nil {
			return ReleaseTrace{}, err
		}
		if err = check(`SELECT workspace_id FROM entity_mapping_decision WHERE id=$1`, v.DecisionID); err != nil {
			return ReleaseTrace{}, err
		}
	}
	for _, v := range t.Evidence {
		if v.WorkspaceID != ws {
			return ReleaseTrace{}, ErrScope
		}
	}
	for _, v := range t.CostEvents {
		if v.WorkspaceID != ws {
			return ReleaseTrace{}, ErrScope
		}
	}
	for _, v := range t.AuditEvents {
		if err = check(`SELECT workspace_id FROM audit_event WHERE id=$1`, v.ID); err != nil {
			return ReleaseTrace{}, err
		}
	}
	memberCount := 0
	if t.EvidenceSnapshot != nil {
		memberCount = len(t.EvidenceSnapshot.Items)
	}
	if len(t.DatasetVersions)+len(t.Executions)+len(t.EntityMappings)+len(t.EntityMatchJobs)+len(t.Evidence)+len(t.CostEvents)+len(t.AuditEvents)+memberCount > 1000 {
		return ReleaseTrace{}, ErrScope
	}
	return t, nil
}

// ExecutionInWorkspace uses the same consistent read boundary as release traces.
func (r *Repository) ExecutionInWorkspace(ctx context.Context, ws, id uuid.UUID) ([]evidence.Item, []cost.Item, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	reader := &Repository{query: tx, evidenceRepo: evidence.NewQueryRepository(tx), costRepo: cost.NewQueryRepository(tx)}
	owner, err := reader.ObjectWorkspace(ctx, "EXECUTION", id)
	if err != nil {
		return nil, nil, err
	}
	if owner != ws {
		return nil, nil, pgx.ErrNoRows
	}
	e, err := reader.evidenceRepo.ListForObject(ctx, "EXECUTION", id)
	if err != nil {
		return nil, nil, err
	}
	c, err := reader.costRepo.ListByExecution(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	for _, v := range e {
		if v.WorkspaceID != ws {
			return nil, nil, ErrScope
		}
	}
	for _, v := range c {
		if v.WorkspaceID != ws {
			return nil, nil, ErrScope
		}
	}
	if len(e)+len(c) > 1000 {
		return nil, nil, ErrScope
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return e, c, nil
}
