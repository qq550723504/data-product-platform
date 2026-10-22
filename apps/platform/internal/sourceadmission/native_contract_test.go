package sourceadmission

import (
	"context"
	"errors"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"testing"
)

type invalidNativeRead struct{ lc.ProtectedObjectPort }

func (p invalidNativeRead) ReadProtected(context.Context, lc.ProtectedObjectIntent) ([]byte, error) {
	return nil, lc.ErrStorageContract
}

// The native adapter detects a retained object's wrong length before the
// producer's own hash comparison. Proven corruption is not transport UNKNOWN.
func TestPostgresNativeCopyContractFailureIsIntegrity(t *testing.T) {
	f := newFixture(t)
	original := f.admit(t)
	v, e := f.s.Store(f.c)
	if e != nil {
		t.Fatal(e)
	}
	store := v.(*evidenceStore)
	store.port = invalidNativeRead{store.port}
	if _, e = f.s.Admit(context.Background(), f.r, f.caller); !errors.Is(e, ErrIntegrity) {
		t.Fatalf("native proven copy corruption must be integrity, got %v", e)
	}
	rec, e := f.s.LookupKey(context.Background(), f.c.WorkspaceID, f.r.Key, f.caller)
	if e != nil || rec.State != "ACCEPTED_SOURCE" || rec.VersionID == nil || *rec.VersionID != *original.VersionID {
		t.Fatalf("corruption rewrote frozen history: %+v %v", rec, e)
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.c.DatasetID) != 1 {
		t.Fatal("corruption created repair version")
	}
}
