package application

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

type FixtureReferenceCount struct {
	Schema, Table, Column string
	Count                 int
}

// Strict closed-fixture adapter: discover all typed dataset_version FK members.
// Own registry and restriction facts describe identities/controls, never bytes
// consumption. Every other relationship conservatively blocks, including a new
// module's FK; no lifecycle owner silently downgrades another owner's history.
func InspectFixtureReferences(ctx context.Context, tx pgx.Tx, c domain.Copy) (domain.Check, []FixtureReferenceCount, error) {
	rows, err := tx.Query(ctx, `SELECT ns.nspname,t.relname,a.attname FROM pg_constraint f JOIN pg_class t ON t.oid=f.conrelid JOIN pg_namespace ns ON ns.oid=t.relnamespace JOIN LATERAL unnest(f.conkey,f.confkey) AS k(local_col,foreign_col) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=k.local_col JOIN pg_attribute b ON b.attrelid=f.confrelid AND b.attnum=k.foreign_col WHERE f.contype='f' AND f.confrelid='dataset_version'::regclass AND b.attname='id' AND t.relname NOT IN ('lifecycle_fixture_copy','lifecycle_content_use_restriction') ORDER BY ns.nspname,t.relname,a.attname`)
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
		query := "SELECT count(*) FROM " + pgx.Identifier{r.Schema, r.Table}.Sanitize() + " WHERE " + pgx.Identifier{r.Column}.Sanitize() + "=$1"
		if err = tx.QueryRow(ctx, query, c.DatasetVersionID).Scan(&r.Count); err != nil {
			return domain.Unknown, nil, err
		}
		if r.Count != 0 {
			check = domain.Denied
		}
	}
	return check, result, nil
}
