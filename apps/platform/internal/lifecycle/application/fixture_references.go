package application

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

type FixtureReferenceCount struct {
	Schema, Table, Column string
	Count                 int
	Unreleased            int
}

// Strict closed-fixture adapter: discover all typed dataset_version FK members.
// Owners release individual exact-copy demands without removing the source FK.
// Unknown relationships or missing admission guards fail closed.
func InspectFixtureReferences(ctx context.Context, tx pgx.Tx, c domain.Copy) (domain.Check, []FixtureReferenceCount, error) {
	if _, err := deliveryfence.Lock(ctx, tx, c.WorkspaceID); err != nil {
		return domain.Unknown, nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM lifecycle_fixture_copy WHERE workspace_id=$1 AND dataset_version_id=$2 ORDER BY id FOR UPDATE`, c.WorkspaceID, c.DatasetVersionID); err != nil {
		return domain.Unknown, nil, err
	}
	// Prevent concurrent FK DDL from changing the discovered closed scope.
	// Ordinary writes still proceed through their workspace/copy admission fence.
	if _, err := tx.Exec(ctx, `LOCK TABLE dataset_version IN SHARE UPDATE EXCLUSIVE MODE`); err != nil {
		return domain.Unknown, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ns.nspname,t.relname,a.attname FROM pg_constraint f JOIN pg_class t ON t.oid=f.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN LATERAL unnest(f.conkey,f.confkey) AS k(local_col,foreign_col) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local_col JOIN pg_attribute b ON b.attrelid=f.confrelid AND b.attnum=k.foreign_col WHERE f.contype='f' AND f.confrelid='dataset_version'::regclass AND b.attname='id' AND NOT (ns.nspname='public' AND a.attname='dataset_version_id' AND t.relname IN ('lifecycle_fixture_copy','lifecycle_content_use_restriction','lifecycle_copy_policy','lifecycle_restore_checkpoint','lifecycle_fixture_reference_requirement')) ORDER BY ns.nspname,t.relname,a.attname`)
	if err != nil {
		return domain.Unknown, nil, err
	}
	var result []FixtureReferenceCount
	for rows.Next() {
		var r FixtureReferenceCount
		if err = rows.Scan(&r.Schema, &r.Table, &r.Column); err != nil {
			rows.Close()
			return domain.Unknown, nil, err
		}
		result = append(result, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return domain.Unknown, nil, err
	}
	check := domain.Satisfied
	for i := range result {
		r := &result[i]
		// Schema evolution is fail closed even when a new table is currently empty.
		var fenced bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger g JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname=$1 AND t.relname=$2 AND g.tgname=$3 AND g.tgenabled='O' AND g.tgfoid='lifecycle_reference_admission_guard()'::regprocedure)`, r.Schema, r.Table, "aa_lifecycle_ref_"+r.Column).Scan(&fenced); err != nil {
			return domain.Unknown, nil, err
		}
		if !fenced {
			check = domain.Unknown
		}
		query := "SELECT count(*) FROM " + pgx.Identifier{r.Schema, r.Table}.Sanitize() + " WHERE " + pgx.Identifier{r.Column}.Sanitize() + "=$1"
		if err = tx.QueryRow(ctx, query, c.DatasetVersionID).Scan(&r.Count); err != nil {
			return domain.Unknown, nil, err
		}
		r.Unreleased = r.Count
		if spec, managed := contentreference.ForRelation(r.Schema, r.Table, r.Column); managed {
			identity := "q." + pgx.Identifier{spec.FactColumn}.Sanitize() + "=s.id"
			if spec.Kind == "EXECUTION_INPUT" {
				identity = "q.execution_id=s.execution_id AND q.input_name=s.input_name"
			}
			query = "SELECT count(*) FROM " + pgx.Identifier{r.Schema, r.Table}.Sanitize() + " s WHERE s." + pgx.Identifier{r.Column}.Sanitize() + "=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement q JOIN lifecycle_fixture_reference_release x ON x.requirement_id=q.id AND x.workspace_id=q.workspace_id WHERE q.workspace_id=$2 AND q.copy_id=$3 AND q.kind=$4 AND " + identity + ")"
			if err = tx.QueryRow(ctx, query, c.DatasetVersionID, c.WorkspaceID, c.ID, spec.Kind).Scan(&r.Unreleased); err != nil {
				return domain.Unknown, nil, err
			}
		}
		if r.Unreleased != 0 && check != domain.Unknown {
			check = domain.Denied
		}
	}
	return check, result, nil
}
