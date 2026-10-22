package application

import (
	"context"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestCheckpointReadNeedsDedicatedCapabilityAndKeepsPhysicalCost(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	w, copies, s, a, _, _, _ := occupancyFixture(t, pool)
	a.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
	at := time.Now().UTC().Truncate(time.Microsecond)
	s.now = func() time.Time { return at.Add(time.Minute) }
	journal := NewPostgresStorageCallJournal(s)
	start := StorageCallStart{ID: uuid.New(), Copy: copies[0], Operation: StorageReadCheckpoint, StartedAt: at}
	if journal.StartStorageCall(ctx, start) == nil {
		t.Fatal("ordinary inspection admitted backup payload")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE id=$1`, start.ID) != 0 {
		t.Fatal("denied start persisted")
	}
	a.caller.Capabilities[CapabilityCheckpointStorage] = struct{}{}
	if e := journal.StartStorageCall(ctx, start); e != nil {
		t.Fatal(e)
	}
	if e := journal.StartStorageCall(ctx, start); e == nil {
		t.Fatal("physical attempt restarted")
	}
	invoked := at.Add(time.Second)
	observation := StorageCallObservation{CallID: start.ID, ObservedAt: at.Add(2 * time.Second), InvokedAt: &invoked, HTTPStatus: 200, ResponseSHA256: strings.Repeat("a", 64)}
	a.denied.Store(true)
	for i := 0; i < 2; i++ {
		if e := journal.ObserveStorageCall(ctx, observation); e != nil {
			t.Fatal(e)
		}
	}
	if count(t, pool, `SELECT count(*) FROM cost_event e JOIN cost_allocation a ON a.cost_event_id=e.id WHERE e.workspace_id=$1 AND a.lifecycle_storage_call_id=$2`, w, start.ID) != 1 {
		t.Fatal("revoked/duplicate observation lost or repeated cost")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reader_pin WHERE copy_id=$1`, copies[0].ID) != 0 {
		t.Fatal("maintenance fabricated business reader")
	}
	a.denied.Store(false)
	bad := start
	bad.ID = uuid.New()
	bad.CopyOperationID = uuid.New()
	if journal.StartStorageCall(ctx, bad) == nil {
		t.Fatal("backup admitted fake custody intent")
	}
}
