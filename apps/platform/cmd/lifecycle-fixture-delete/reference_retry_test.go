package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestReferenceNewExecutionRejectsOldPermissionAndProfile(t *testing.T) {
	if pilot.ReferenceExecution == "614cd9ad-3f9d-40be-b110-3f7cf95f0790" {
		t.Fatal("consumed execution reused")
	}
	at := time.Now().UTC()
	c := referenceConfig{}
	c.Fixture.InspectAccess = "inspectfixture"
	p := referencePermission{permission: permission{Enabled: true, RunID: uuid.MustParse("614cd9ad-3f9d-40be-b110-3f7cf95f0790"), DeleteAccess: "refdelete614cd9ad"}}
	if validateReferencePermission(p, c, at) == nil {
		t.Fatal("old authority accepted")
	}
	t.Setenv("LIFECYCLE_REFERENCE_PROFILE", "5d2274ee-614cd9ad")
	if runReference(context.Background(), "render") == nil {
		t.Fatal("old profile accepted")
	}
}

func TestReferenceRetryRequiresClosedDenialAndSingleAttemptOrdering(t *testing.T) {
	for _, stage := range []string{"send-a", "send-b"} {
		absent := stage == "send-b"
		if validateReferenceSendHistory(stage, true, true, false, absent) != nil {
			t.Fatal("verified prior closure rejected")
		}
		for _, state := range [][4]bool{{false, true, false, absent}, {true, false, false, absent}, {true, true, true, absent}, {true, true, false, !absent}} {
			if validateReferenceSendHistory(stage, state[0], state[1], state[2], state[3]) == nil {
				t.Fatal("missing closure, repeat or out-of-order send accepted", stage, state)
			}
		}
	}
	if validateReferenceSendHistory("foreign", true, true, false, false) == nil {
		t.Fatal("foreign stage accepted")
	}
}
