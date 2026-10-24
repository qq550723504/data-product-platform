package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const sixReadPurpose = "SIX_TARGET_PREPARATION_NATIVE_READONLY"

type sixReadProfile struct {
	postV22ReadProfile
	Execution, Profile, Role, MapSHA256 string
}

type sixEnvironmentMap = pilot.RemainingEnvironmentMap

func parseSixReadProfile(raw, mapping []byte, at time.Time) (postV22ReadProfile, error) {
	return parseSixMaintenanceProfile(raw, mapping, at, sixReadPurpose)
}
func parseSixMaintenanceProfile(raw, mapping []byte, at time.Time, purpose string) (postV22ReadProfile, error) {
	var p sixReadProfile
	var m sixEnvironmentMap
	if strictPostV22(raw, &p) != nil || strictPostV22(mapping, &m) != nil {
		return p.postV22ReadProfile, errors.New("strict six read-only profile/map required")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 21 || string(fields["MaxProviderDeletes"]) != "0" {
		return p.postV22ReadProfile, errors.New("six explicit zero-delete profile fields required")
	}
	if p.Execution != pilot.RemainingExecution || m.Execution != p.Execution || p.Profile != pilot.RemainingProfile || m.Profile != p.Profile ||
		p.Purpose != purpose || !p.Enabled || p.MaxProviderDeletes != 0 || p.AuditID.String() == p.Execution ||
		p.AuditID.String() == "00000000-0000-0000-0000-000000000000" || p.MapSHA256 != postV22Hash(mapping) || p.CandidateSHA256 != p.MapSHA256 ||
		!hexDigest(p.BinarySHA256) || !hexDigest(p.PresentInventorySHA256) || p.Endpoint != "127.0.0.1:9000" || p.RootAccess == "" || p.RootSecret == "" ||
		p.NotBefore.IsZero() || p.ExpiresAt.Sub(p.NotBefore) <= 0 || p.ExpiresAt.Sub(p.NotBefore) > 55*time.Minute || at.Before(p.NotBefore) || !at.Before(p.ExpiresAt) ||
		(p.Role != "SOURCE" && p.Role != "RESTORE") || len(m.Groups) != 3 {
		return p.postV22ReadProfile, errors.New("six read-only scope/lifetime differs")
	}
	source, known := postV22Sources[p.Group]
	if !known || p.FrozenInventorySHA256 != source[3] || !m.Matches(p.Group, p.Role, [3]string{p.Core, p.Native, p.Network}) {
		return p.postV22ReadProfile, errors.New("six actual environment role differs")
	}
	return p.postV22ReadProfile, nil
}

func sixVerifyNative(ctx context.Context) error {
	raw, err := os.ReadFile("/run/private/six-readonly.json")
	if err != nil {
		return err
	}
	mapping, err := os.ReadFile("/run/private/six-map.json")
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	p, err := parseSixReadProfile(raw, mapping, started)
	if err != nil {
		return err
	}
	return verifyNativeAudit(ctx, p, raw, started, "six-readonly.json", "six-native-verification.json", true)
}
