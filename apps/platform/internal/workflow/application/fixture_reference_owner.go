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

func (FixtureReferenceOwner) Module() string { return "workflow" }
func (FixtureReferenceOwner) Check(ctx context.Context, tx pgx.Tx, workspace, version uuid.UUID, ref contentreference.Reference) (contentreference.Spec, error) {
	if ref.Validate() != nil {
		return contentreference.Spec{}, contentreference.ErrReference
	}
	spec, ok := contentreference.Lookup(ref.Kind)
	if !ok || spec.Module != "workflow" {
		return spec, contentreference.ErrReference
	}
	var valid bool
	var err error
	switch ref.Kind {
	case "EXECUTION_INPUT":
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_input r JOIN execution e ON e.id=r.execution_id WHERE r.execution_id=$1 AND r.dataset_version_id=$2 AND e.workspace_id=$3 AND r.input_name=$4)`, ref.SubjectID, version, workspace, ref.InputName).Scan(&valid)
	case "EXECUTION_MAPPING":
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_mapping_usage r  WHERE r.id=$1 AND r.input_dataset_version_id=$2 AND r.workspace_id=$3)`, ref.SubjectID, version, workspace).Scan(&valid)
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
