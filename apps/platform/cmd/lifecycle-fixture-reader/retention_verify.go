package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"os"
	"reflect"
	"time"
)

const retentionReadPurpose = "RETENTION_MAINTENANCE_NATIVE_READONLY"

type retentionReadProfile struct {
	postV22ReadProfile
	MaintenanceID                                   uuid.UUID
	Phase, BindingsSHA256, EffectiveInventorySHA256 string
}

func parseRetentionReadProfile(raw []byte, at time.Time) (retentionReadProfile, error) {
	var p retentionReadProfile
	var fields map[string]json.RawMessage
	if sixUniqueJSON(raw) != nil || strictPostV22(raw, &p) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 21 || string(fields["MaxProviderDeletes"]) != "0" {
		return p, errors.New("explicit maintenance read-only profile required")
	}
	source, known := postV22Sources[p.Group]
	if !known || !p.Enabled || p.Purpose != retentionReadPurpose || p.MaintenanceID == uuid.Nil || p.MaintenanceID.String() == pilot.RemainingExecution || p.AuditID == uuid.Nil || p.AuditID == p.MaintenanceID ||
		(p.Phase != "before" && p.Phase != "after") || p.CandidateSHA256 != p.BindingsSHA256 || !hexDigest(p.BindingsSHA256) || !hexDigest(p.EffectiveInventorySHA256) ||
		!hexDigest(p.BinarySHA256) || !hexDigest(p.PresentInventorySHA256) || p.Core != source[0] || p.Native != source[1] || p.Network != source[2] || p.FrozenInventorySHA256 != source[3] ||
		p.Endpoint != "127.0.0.1:9000" || p.RootAccess == "" || p.RootSecret == "" || p.NotBefore.IsZero() || !p.NotBefore.Before(p.ExpiresAt) || p.ExpiresAt.Sub(p.NotBefore) > 55*time.Minute || at.Before(p.NotBefore) || !at.Before(p.ExpiresAt) {
		return p, errors.New("maintenance read-only identity/phase/window differs")
	}
	return p, nil
}

func retentionEffectiveInventory(p retentionReadProfile, actual []remainingExpected, effective []byte) ([]remainingExpected, error) {
	expected, err := pilot.RetentionMaintenanceInventory(p.Group, p.Phase, actual)
	var supplied []remainingExpected
	if err != nil || postV22Hash(effective) != p.EffectiveInventorySHA256 || sixUniqueJSON(effective) != nil || strictPostV22(effective, &supplied) != nil || !reflect.DeepEqual(expected, supplied) {
		return nil, errors.New("maintenance effective inventory differs from exact-two phase")
	}
	return expected, nil
}

func retentionVerifyNative(ctx context.Context) error {
	raw, err := os.ReadFile("/run/private/retention-readonly.json")
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	p, err := parseRetentionReadProfile(raw, started)
	if err != nil {
		return err
	}
	// Old inventory/P1 validation always runs first inside the common entry.
	return verifyNativeAuditPhase(ctx, p.postV22ReadProfile, raw, started, "retention-readonly.json", "retention-native-verification.json", true, &p)
}
