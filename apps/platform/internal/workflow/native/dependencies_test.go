package native

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

func TestRestorePreparedDependenciesAllowsZeroMappingUsage(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve native package path")
	}
	industryPackRoot := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "..", "industry-packs")
	companyContent, err := os.ReadFile(filepath.Join(industryPackRoot, "park", "matching", "company-match-policy-v1.yaml"))
	if err != nil {
		t.Fatalf("read company policy: %v", err)
	}
	indicatorContent, err := os.ReadFile(filepath.Join(industryPackRoot, "park", "indicators", "enterprise-activity-v1.yaml"))
	if err != nil {
		t.Fatalf("read indicator policy: %v", err)
	}
	resolutionContent := []byte("resolution dependency snapshot")
	resolutionVersionID := uuid.New()
	preparation := workflowdomain.DependencyPreparation{
		ExecutionID:        uuid.New(),
		WorkspaceID:        uuid.New(),
		BindingFingerprint: hashBytes([]byte("zero-usage-preparation")),
		MappingUsageCount:  0,
		Status:             "PREPARED",
		Dependencies: []workflowdomain.DependencyBinding{
			{Name: enterpriseResolutionInput, DatasetVersionID: &resolutionVersionID, ContentSHA256: hashBytes(resolutionContent), Content: resolutionContent},
			{Name: companyPolicyDependency, ContentSHA256: hashBytes(companyContent), Content: companyContent},
			{Name: indicatorPolicyDependency, ContentSHA256: hashBytes(indicatorContent), Content: indicatorContent},
		},
	}

	prepared, err := (&Engine{}).restorePreparedDependencies(preparation, resolutionVersionID)
	if err != nil {
		t.Fatalf("restore zero-usage preparation: %v", err)
	}
	if len(prepared.Mappings) != 0 {
		t.Fatalf("restored mapping count = %d, want 0", len(prepared.Mappings))
	}
}
