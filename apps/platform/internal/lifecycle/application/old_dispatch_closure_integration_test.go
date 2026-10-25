package application

import (
	"context"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Full migrated PostgreSQL/Command composition behind the host's exact delta.
// The provider verifier here is synthetic; the signed adapter's SQL eligibility
// contract is tested separately in pilot. No storage provider is configured.
func TestOldDispatchClosureThreeCommandCompositionPreservesFacts(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	fixtures := []*disposalFixture{newDisposalFixture(t, pool), newDisposalFixture(t, pool, true), newDisposalFixture(t, pool)}
	// Establish synthetic historical termination before the tested closure. The
	// retained pin row is then part of the immutable baseline, just as live T1.
	f := fixtures[1]
	f.now = f.now.Add(time.Minute)
	if _, err := f.o.TerminateFixtureReader(ctx, TerminateFixtureReaderCommand{CommandHeader: f.header("historical-reader-completed"), PinID: f.reader.ID, Generation: f.reader.Generation, Outcome: domain.ReaderCompleted}); err != nil {
		t.Fatal(err)
	}
	attempts := make([]DispatchResult, 0, 3)
	for _, f := range fixtures {
		attempts = append(attempts, requireDispatch(t, f, "old-dispatch", 0))
	}
	expected := map[string]int{"lifecycle_approval_revocation": 3, "lifecycle_provider_observation": 3, "lifecycle_dispatch_release": 3,
		"command_idempotency": 6, "audit_event": 9, "evidence": 9, "evidence_relation": 9, "outbox_event": 9}
	before := map[string]int{}
	for table := range expected {
		before[table] = count(t, pool, "SELECT count(*) FROM "+table)
	}
	stable := map[string]string{}
	for _, table := range []string{"lifecycle_provider_attempt", "lifecycle_disposition_dispatch", "lifecycle_disposition_approval", "lifecycle_reader_pin", "lifecycle_provider_send_claim", "cost_event", "cost_allocation"} {
		var digest string
		if err := pool.QueryRow(ctx, "SELECT md5(coalesce(string_agg(to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text),'')) FROM "+table+" t").Scan(&digest); err != nil {
			t.Fatal(err)
		}
		stable[table] = digest
	}
	for i, f := range fixtures {
		f.now = f.now.Add(time.Minute)
		cmd := RevokeDispositionApprovalCommand{CommandHeader: f.header("closure-revoke"), ApprovalID: f.approval.ID, BasisRef: "new-non-delete-closure"}
		first, err := f.s.RevokeDispositionApproval(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := f.s.RevokeDispositionApproval(ctx, cmd)
		if err != nil || !replay.Replayed || replay.ID != first.ID {
			t.Fatal("revocation replay", replay, err)
		}
		observe := RecordProviderObservationCommand{CommandHeader: f.header("closure-NOT_SENT"), AttemptID: attempts[i].AttemptID, ObservationRef: "not-sent"}
		first, err = f.s.RecordProviderObservation(ctx, observe)
		if err != nil {
			t.Fatal(err)
		}
		replay, err = f.s.RecordProviderObservation(ctx, observe)
		if err != nil || !replay.Replayed || replay.ID != first.ID {
			t.Fatal("observation replay", replay, err)
		}
	}
	for table, delta := range expected {
		if actual := count(t, pool, "SELECT count(*) FROM "+table) - before[table]; actual != delta {
			t.Fatalf("%s delta=%d expected=%d", table, actual, delta)
		}
	}
	for table, expected := range stable {
		var actual string
		if err := pool.QueryRow(ctx, "SELECT md5(coalesce(string_agg(to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text),'')) FROM "+table+" t").Scan(&actual); err != nil || actual != expected {
			t.Fatalf("old %s changed: %v", table, err)
		}
	}
}

// A new execution is a new approval/dispatch/attempt. Closing the old attempt
// neither reactivates its revoked approval nor rewrites its immutable rows.
func TestOldDispatchClosureAllowsOnlyFreshApprovalAndAttempt(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	f := newDisposalFixture(t, pool)
	old := requireDispatch(t, f, "old-execution-dispatch", 0)
	oldApproval := f.approval
	stable := map[string]string{}
	for _, table := range []string{"lifecycle_provider_attempt", "lifecycle_disposition_dispatch", "lifecycle_disposition_approval"} {
		var row string
		if err := pool.QueryRow(ctx, "SELECT to_jsonb(t)::text FROM "+table+" t LIMIT 1").Scan(&row); err != nil {
			t.Fatal(err)
		}
		stable[table] = row
	}
	freshApproval, err := f.s.ApproveContentDisposition(ctx, ApproveContentDispositionCommand{CommandHeader: f.header("new-execution-approve"), PlanID: f.plan.ID, BasisRef: "new-independent-execution", ExpiresAt: f.now.Add(time.Hour)})
	if err != nil || freshApproval.ID == oldApproval.ID {
		t.Fatal("new approval", freshApproval, err)
	}
	f.approval = freshApproval
	blocked, err := f.s.DispatchContentDisposition(ctx, f.dispatch("new-while-old-pending", 0))
	if err != nil || blocked.Admitted || !hasBlocker(blocked, "DISPOSITION_ALREADY_DISPATCHED_OR_UNKNOWN") {
		t.Fatal("new approval bypassed pending old attempt", blocked, err)
	}
	f.now = f.now.Add(time.Minute)
	if _, err = f.s.RevokeDispositionApproval(ctx, RevokeDispositionApprovalCommand{CommandHeader: f.header("old-execution-revoke"), ApprovalID: oldApproval.ID, BasisRef: "trusted-closure"}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RecordProviderObservation(ctx, RecordProviderObservationCommand{CommandHeader: f.header("old-execution-NOT_SENT"), AttemptID: old.AttemptID, ObservationRef: "not-sent"}); err != nil {
		t.Fatal(err)
	}
	f.approval = oldApproval
	blocked, err = f.s.DispatchContentDisposition(ctx, f.dispatch("old-approval-after-closure", 0))
	if err != nil || blocked.Admitted || !hasBlocker(blocked, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
		t.Fatal("NOT_SENT reactivated revoked approval", blocked, err)
	}
	f.approval = freshApproval
	fresh := requireDispatch(t, f, "new-after-trusted-closure", 0)
	if fresh.AttemptID == old.AttemptID || fresh.DispatchID == old.DispatchID {
		t.Fatal("new execution reused an old fact")
	}
	for table, row := range stable {
		if count(t, pool, "SELECT count(*) FROM "+table+" t WHERE to_jsonb(t)::text=$1", row) != 1 {
			t.Fatalf("old %s row changed", table)
		}
	}
	for _, table := range []string{"lifecycle_provider_send_claim", "cost_event", "cost_allocation"} {
		if count(t, pool, "SELECT count(*) FROM "+table) != 0 {
			t.Fatalf("non-delete admission unexpectedly invoked provider: %s", table)
		}
	}
	if count(t, pool, "SELECT count(*) FROM lifecycle_dispatch_release WHERE dispatch_id=$1", old.DispatchID) != 1 || count(t, pool, "SELECT count(*) FROM lifecycle_provider_observation WHERE attempt_id=$1 AND certainty='NOT_SENT'", old.AttemptID) != 1 {
		t.Fatal("trusted closure facts missing")
	}
}
