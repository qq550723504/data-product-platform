package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
)

// FixtureReferenceOwner is explicitly installed for closed synthetic fixtures.
// Loss is an owner decision: the immutable business result remains historical,
// while source replay/content revalidation is unavailable after approved release.
// No terminal status or expired lease is treated as stopped-I/O proof.
type FixtureReferenceOwner struct{}

func (FixtureReferenceOwner) Module() string { return "certification" }
func (FixtureReferenceOwner) Check(ctx context.Context, tx pgx.Tx, workspace, version uuid.UUID, ref contentreference.Reference) (contentreference.Spec, error) {
	if ref.Validate() != nil {
		return contentreference.Spec{}, contentreference.ErrReference
	}
	spec, ok := contentreference.Lookup(ref.Kind)
	if !ok || spec.Module != "certification" {
		return spec, contentreference.ErrReference
	}
	var valid bool
	var err error
	switch ref.Kind {
	case "CERTIFICATION":
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_certification r  WHERE r.id=$1 AND r.dataset_version_id=$2 AND r.workspace_id=$3)`, ref.SubjectID, version, workspace).Scan(&valid)
	default:
		return spec, contentreference.ErrReference
	}
	if err != nil {
		return spec, err
	}
	if !valid {
		return spec, contentreference.ErrReference
	}
	return spec, nil
}
