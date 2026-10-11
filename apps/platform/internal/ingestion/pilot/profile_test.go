package pilot

import (
	"testing"

	"github.com/google/uuid"
	adapter "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/hop"
)

func TestPrivateTargetBindingsFailClosed(t *testing.T) {
	ws := uuid.New()
	p := Profile{}
	for _, code := range []string{"enterprise_raw", "lease_raw", "energy_raw"} {
		p.Tables = append(p.Tables, adapter.Table{Code: code})
		p.Targets = append(p.Targets, TargetBinding{WorkspaceID: ws, Code: code, ResourceID: uuid.New(), DatasetID: uuid.New()})
	}
	if targets, err := configuredTargets(p, ws); err != nil || len(targets) != 3 {
		t.Fatal("valid operator bindings rejected")
	}
	if _, err := configuredTargets(p, uuid.New()); err == nil {
		t.Fatal("bindings exposed to another workspace")
	}
	missing := p
	missing.Targets = nil
	if _, err := configuredTargets(missing, ws); err == nil {
		t.Fatal("missing trusted bindings inferred from names")
	}
	p.Targets[1].ResourceID = p.Targets[0].ResourceID
	if _, err := configuredTargets(p, ws); err == nil {
		t.Fatal("shared resource accepted")
	}
}
