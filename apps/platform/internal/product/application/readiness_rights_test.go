package application

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestEvaluateRightsReadiness(t *testing.T) {
	resourceID := uuid.New()

	tests := []struct {
		name         string
		facts        infrastructure.ReadinessFacts
		wantStatus   CheckStatus
		wantBlockers []string
	}{
		{
			name:         "missing snapshot",
			facts:        infrastructure.ReadinessFacts{},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_SNAPSHOT_MISSING"},
		},
		{
			name: "inactive rights",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_INVALID"},
		},
		{
			name: "snapshot bound to another release",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_SNAPSHOT_RELEASE_MISMATCH"},
		},
		{
			name: "snapshot not finalized",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_SNAPSHOT_NOT_FINALIZED"},
		},
		{
			name: "snapshot context mismatch",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsCurrentlyValid:         true,
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_CONTEXT_MISMATCH"},
		},
		{
			name: "missing upstream resource",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
				RightsCoverageKnown:          true,
				MissingResourceIDs:           []uuid.UUID{resourceID},
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_RESOURCE_NOT_COVERED"},
		},
		{
			name: "missing productize action",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
				RightsCoverageKnown:          true,
				MissingActions:               map[string][]string{resourceID.String(): {"PRODUCTIZE"}},
			},
			wantStatus:   CheckFail,
			wantBlockers: []string{"RIGHTS_ACTION_NOT_COVERED"},
		},
		{
			name: "legacy lineage with no source resource",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
			},
			wantStatus: CheckPass,
		},
		{
			name: "complete lineage coverage",
			facts: infrastructure.ReadinessFacts{
				RightsSnapshotExists:         true,
				RightsSnapshotWorkspaceMatch: true,
				RightsSnapshotReleaseMatch:   true,
				RightsSnapshotFinalized:      true,
				RightsSnapshotContextMatch:   true,
				RightsCurrentlyValid:         true,
				RightsCoverageKnown:          true,
				RightsCoverageComplete:       true,
				RequiredResourceIDs:          []uuid.UUID{resourceID},
			},
			wantStatus: CheckPass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, blockers, details := evaluateRightsReadiness(tt.facts)
			if status != tt.wantStatus {
				t.Fatalf("status = %s, want %s", status, tt.wantStatus)
			}
			if len(blockers) != len(tt.wantBlockers) {
				t.Fatalf("blockers = %v, want %v", blockers, tt.wantBlockers)
			}
			for _, blocker := range tt.wantBlockers {
				if !slices.Contains(blockers, blocker) {
					t.Fatalf("blockers = %v, missing %s", blockers, blocker)
				}
			}
			if details == nil {
				t.Fatal("rights readiness details are nil")
			}
		})
	}
}
