package application

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestReadinessProductionRequiresProducingExecution(t *testing.T) {
	target := uuid.New()
	result := readinessResultFromFacts(uuid.New(), infrastructure.ReadinessFacts{
		TargetDatasetVersionID: &target,
		AllDatasetsUsable:      true,
	})

	if result.Checks["production"] != CheckFail {
		t.Fatalf("production check = %s, want FAIL", result.Checks["production"])
	}
	if !slices.Contains(result.Blockers, "PRODUCTION_EXECUTION_MISSING") {
		t.Fatalf("blockers = %v, want PRODUCTION_EXECUTION_MISSING", result.Blockers)
	}
}

func TestDraftReadinessDoesNotClaimProductionWithoutExecution(t *testing.T) {
	target := uuid.New()
	result := draftReadinessResultFromFacts(uuid.New(), infrastructure.ReadinessFacts{
		TargetDatasetVersionID: &target,
		AllDatasetsUsable:      true,
	})

	if result.Checks["production"] != CheckFail {
		t.Fatalf("draft production check = %s, want FAIL", result.Checks["production"])
	}
	if result.Checks["rights"] != CheckPending {
		t.Fatalf("draft rights check = %s, want PENDING", result.Checks["rights"])
	}
	if !slices.Contains(result.Blockers, "PRODUCTION_EXECUTION_MISSING") {
		t.Fatalf("draft blockers = %v, want PRODUCTION_EXECUTION_MISSING", result.Blockers)
	}
}


func TestReadinessProductionRejectsWorkflowMismatch(t *testing.T) {
	target := uuid.New()
	result := readinessResultFromFacts(uuid.New(), infrastructure.ReadinessFacts{
		TargetDatasetVersionID:     &target,
		AllDatasetsUsable:          true,
		ProductionExecutionPresent: true,
		ProductionWorkflowMatch:    false,
		ProductionLineageComplete:  true,
	})

	if !slices.Contains(result.Blockers, "PRODUCTION_WORKFLOW_MISMATCH") {
		t.Fatalf("blockers = %v, want PRODUCTION_WORKFLOW_MISMATCH", result.Blockers)
	}
}

func TestReadinessProductionRejectsMissingLineage(t *testing.T) {
	target := uuid.New()
	result := readinessResultFromFacts(uuid.New(), infrastructure.ReadinessFacts{
		TargetDatasetVersionID:     &target,
		AllDatasetsUsable:          true,
		ProductionExecutionPresent: true,
		ProductionWorkflowMatch:    true,
		ProductionLineageComplete:  false,
	})

	if !slices.Contains(result.Blockers, "PRODUCTION_LINEAGE_INCOMPLETE") {
		t.Fatalf("blockers = %v, want PRODUCTION_LINEAGE_INCOMPLETE", result.Blockers)
	}
}
