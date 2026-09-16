package application

import "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"

func evaluateRightsReadiness(facts infrastructure.ReadinessFacts) (CheckStatus, []string, map[string]any) {
	details := map[string]any{
		"snapshotExists":           facts.RightsSnapshotExists,
		"workspaceMatch":           facts.RightsSnapshotWorkspaceMatch,
		"currentlyValid":           facts.RightsCurrentlyValid,
		"coverageKnown":            facts.RightsCoverageKnown,
		"coverageComplete":         facts.RightsCoverageComplete,
		"requiredResourceIds":      facts.RequiredResourceIDs,
		"missingResourceIds":       facts.MissingResourceIDs,
		"missingActionsByResource": facts.MissingActions,
	}

	if !facts.RightsSnapshotExists {
		return CheckFail, []string{"RIGHTS_SNAPSHOT_MISSING"}, details
	}
	if !facts.RightsSnapshotWorkspaceMatch || !facts.RightsCurrentlyValid {
		return CheckFail, []string{"RIGHTS_INVALID"}, details
	}
	if facts.RightsCoverageKnown && len(facts.MissingResourceIDs) > 0 {
		blockers := []string{"RIGHTS_RESOURCE_NOT_COVERED"}
		if len(facts.MissingActions) > 0 {
			blockers = append(blockers, "RIGHTS_ACTION_NOT_COVERED")
		}
		return CheckFail, blockers, details
	}
	if facts.RightsCoverageKnown && len(facts.MissingActions) > 0 {
		return CheckFail, []string{"RIGHTS_ACTION_NOT_COVERED"}, details
	}
	return CheckPass, nil, details
}
