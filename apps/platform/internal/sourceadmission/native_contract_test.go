package sourceadmission

import (
	"context"
	"errors"
	"testing"

	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

type invalidNativeRead struct{ lc.ProtectedObjectPort }

func (p invalidNativeRead) ReadProtected(context.Context, lc.ProtectedObjectIntent) ([]byte, error) {
	return nil, lc.ErrStorageContract
}

type journalContractRead struct{ lc.ProtectedObjectPort }

func (p journalContractRead) ReadProtected(context.Context, lc.ProtectedObjectIntent) ([]byte, error) {
	return nil, errors.Join(lc.ErrProtectedJournal, lc.ErrStorageContract)
}

func TestPostgresNativeJournalContractFailureKeepsOriginalKeyRecoverable(t *testing.T) {
	f := newFixture(t)
	v, e := f.s.Store(f.c)
	if e != nil {
		t.Fatal(e)
	}
	store := v.(*evidenceStore)
	originalPort := store.port
	store.port = journalContractRead{originalPort}
	_, e = f.s.Admit(context.Background(), f.r, f.caller)
	prepared, lookupErr := f.s.LookupKey(context.Background(), f.c.WorkspaceID, f.r.Key, f.caller)
	if !errors.Is(e, ErrUnknown) || lookupErr != nil || prepared.State != "PREPARED" {
		t.Fatalf("journal uncertainty poisoned original key: error=%v state=%s lookup=%v", e, prepared.State, lookupErr)
	}
	var reserved string
	if e = f.pool.QueryRow(context.Background(), `SELECT version_id::text FROM source_admission_capture WHERE dataset_id=$1`, f.c.DatasetID).Scan(&reserved); e != nil {
		t.Fatal(e)
	}
	store.port = originalPort
	accepted := f.admit(t)
	if accepted.ID != prepared.ID || accepted.VersionID.String() != reserved || f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.c.DatasetID) != 1 {
		t.Fatal("original key did not resume its reserved version")
	}
	for _, query := range []string{
		`SELECT count(*) FROM source_admission_call WHERE workspace_id=$1`,
		`SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='SOURCE_ADMISSION_FETCH' AND quantity=1 AND unit='invocation'`,
		`SELECT count(*) FROM cost_allocation a JOIN source_admission_call c ON c.id=a.source_admission_call_id WHERE c.workspace_id=$1`,
	} {
		if f.count(t, query, f.c.WorkspaceID) != 6 {
			t.Fatal("unknown attempt and recovery did not retain independent invocation costs")
		}
	}
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
