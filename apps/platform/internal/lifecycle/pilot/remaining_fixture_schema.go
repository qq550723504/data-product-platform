package pilot

// This gate belongs only to the retained D1 fixture artifact, not to product P.
// R has one fixed schema contract. It never chooses a repository by schema.
import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type remainingSchemaReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func ValidateRemainingFixtureSchema(ctx context.Context, db remainingSchemaReader) error {
	var maxVersion, members int
	var protectedIdentity bool
	err := db.QueryRow(ctx, `SELECT COALESCE(MAX(version),0), COUNT(*),
 EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public'
 AND table_name='lifecycle_fixture_copy' AND column_name='identity_kind')
 FROM schema_migration`).Scan(&maxVersion, &members, &protectedIdentity)
	if err != nil || maxVersion != 69 || members != 66 || protectedIdentity {
		return errors.New("retained D1 fixture requires exact schema 69/66 without protected identity model; no business or provider operation permitted")
	}
	return nil
}
