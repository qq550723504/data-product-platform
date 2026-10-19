package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestRemainingRenewRejectsWrongCandidateDeadlineBeforeDB(t *testing.T) {
	end, err := time.Parse(time.RFC3339, pilot.RemainingNotAfter)
	if err != nil {
		t.Fatal(err)
	}
	heads := map[string]uuid.UUID{}
	for i := 0; i < 6; i++ {
		heads[uuid.NewString()] = uuid.New()
	}
	for _, until := range []time.Time{{}, end.Add(-time.Second), end.Add(time.Second), end.Add(-24 * time.Hour)} {
		r := remainingConfig{Group: "business-races", CandidateUseUntil: until, ExpectedPolicyHeads: heads}
		// Nil pool is intentional: any DB lookup, marker or Renew before this
		// guard would panic. The strict runtime guard must remain fail closed.
		if got := remainingFreezePolicy(context.Background(), r, nil); got != lc.ErrStorageScope {
			t.Fatalf("deadline %s: got %v", until, got)
		}
	}
}

func TestResumeRejectsStaleCurrentPolicyCheckpointBeforeDatabase(t *testing.T) {
	heads := pilot.RemainingPolicyHeads("business-races")
	for version := range heads {
		heads[version] = uuid.New()
		break
	}
	r := remainingConfig{Group: "business-races", Batch: uuid.MustParse(pilot.RemainingExecution), ExpectedPolicyHeads: heads}
	if got := remainingResumeBusiness(context.Background(), r, nil); got != lc.ErrStorageScope {
		t.Fatalf("stale expected head reached Core or admission: %v", got)
	}
}
