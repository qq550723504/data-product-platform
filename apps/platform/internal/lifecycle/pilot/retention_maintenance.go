package pilot

import (
	"errors"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"time"
)

// These finite limits bound a new human-reviewed exact-two proposal. They do
// not authorize an execution or choose an operational date.
func CheckRetentionMaintenanceHorizon(end, until time.Time) error {
	if end.IsZero() || until.IsZero() || end.Nanosecond() != 0 || until.Nanosecond() != 0 || until.Before(end.Add(2*time.Hour)) || until.After(end.Add(72*time.Hour)) {
		return errors.New("retention horizon must cover two-hour recovery and be at most72 hours")
	}
	return nil
}

// RetentionInventory is the existing native receipt row. Maintenance changes
// its expectation, never the historical receipt or the identity of a Copy.
type RetentionInventory struct {
	Copy        domain.Copy
	LegalHold   bool
	Retention   string
	RetainUntil *time.Time
}

func RetentionMaintenanceInventory(group, phase string, original []RetentionInventory, until time.Time) ([]RetentionInventory, error) {
	if (phase != "before" && phase != "after") || until.IsZero() || until.Nanosecond() != 0 {
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
		if !until.After(*row.RetainUntil) {
			return nil, errors.New("retention maintenance must extend the exact old expiry")
		}
		found++
		if phase == "after" {
			copyUntil := until.UTC()
			output[n].RetainUntil = &copyUntil
		}
	}
	if (version != "" && found != 1) || (version == "" && group != "late-hold") {
		return nil, errors.New("retention maintenance exact membership differs")
	}
	return output, nil
}
