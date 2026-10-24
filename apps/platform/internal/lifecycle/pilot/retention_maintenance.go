package pilot

import (
	"errors"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"time"
)

const RetentionMaintenanceDeadline = "2026-10-24T12:40:00Z"

// RetentionInventory is the existing native receipt row. Maintenance changes
// its expectation, never the historical receipt or the identity of a Copy.
type RetentionInventory struct {
	Copy        domain.Copy
	LegalHold   bool
	Retention   string
	RetainUntil *time.Time
}

func RetentionMaintenanceInventory(group, phase string, original []RetentionInventory) ([]RetentionInventory, error) {
	if phase != "before" && phase != "after" {
		return nil, errors.New("unknown retention maintenance phase")
	}
	scope, err := RemainingScopeFor(group)
	if err != nil {
		return nil, err
	}
	version, old := "", ""
	switch group {
	case "business-races":
		version, old = "2606d71f-207b-42a7-b671-4b7f4ca4d194", "2026-10-23T06:25:44Z"
	case "mixed":
		version, old = "81a8402e-390f-4247-9d34-552df5eadeb3", "2026-10-23T06:33:33Z"
	}
	var target domain.Copy
	for _, cp := range scope.Copies {
		if cp.ProviderVersion == version {
			target = cp
		}
	}
	output := append([]RetentionInventory{}, original...)
	found := 0
	for n, row := range output {
		if version == "" || row.Copy.ID != target.ID {
			continue
		}
		if row.Copy != target || row.LegalHold || row.Retention != "GOVERNANCE" || row.RetainUntil == nil || row.RetainUntil.UTC().Format(time.RFC3339Nano) != old {
			return nil, errors.New("retention maintenance exact old identity/protection differs")
		}
		found++
		if phase == "after" {
			until, _ := time.Parse(time.RFC3339, RetentionMaintenanceDeadline)
			output[n].RetainUntil = &until
		}
	}
	if (version != "" && found != 1) || (version == "" && group != "late-hold") {
		return nil, errors.New("retention maintenance exact membership differs")
	}
	return output, nil
}
