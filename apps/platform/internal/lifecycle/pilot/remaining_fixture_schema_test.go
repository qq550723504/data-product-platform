package pilot

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type schemaRow struct {
	max, count int
	protected  bool
	err        error
}

func (r schemaRow) Scan(dst ...any) error {
	if r.err != nil {
		return r.err
	}
	*dst[0].(*int), *dst[1].(*int), *dst[2].(*bool) = r.max, r.count, r.protected
	return nil
}

type schemaReader struct{ row schemaRow }

func (r schemaReader) QueryRow(context.Context, string, ...any) pgx.Row { return r.row }
func TestRemainingFixtureSchemaSingleContract(t *testing.T) {
	for _, row := range []schemaRow{{69, 66, false, nil}, {68, 65, false, nil}, {70, 67, true, nil}, {71, 68, true, nil}, {69, 66, true, nil}, {69, 65, false, nil}, {69, 66, false, errors.New("unreadable")}} {
		err := ValidateRemainingFixtureSchema(context.Background(), schemaReader{row})
		want := row.max == 69 && row.count == 66 && !row.protected && row.err == nil
		if (err == nil) != want {
			t.Fatalf("schema %+v accepted=%v", row, err == nil)
		}
	}
}
func TestRemainingFixtureSchemaNativePostgres(t *testing.T) {
	dsn := os.Getenv("D1_SCHEMA_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated retained-schema clone only")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	err = ValidateRemainingFixtureSchema(ctx, pool)
	want := os.Getenv("D1_SCHEMA_EXPECT") == "ALLOW"
	if (err == nil) != want {
		t.Fatalf("schema gate allowed=%v expected=%v error=%v", err == nil, want, err)
	}
}
