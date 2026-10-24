package main

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"os"
	"path/filepath"
	"time"
)

type retentionExpectation struct {
	postV22Expectation
	MaintenanceID, Phase, BindingsSHA256 string
	RetainUntil, MaintenanceEnd          time.Time
}

func retentionPhaseRows(group, phase string, rows []json.RawMessage, until time.Time) ([]json.RawMessage, error) {
	original := []pilot.RetentionInventory{}
	for _, raw := range rows {
		var row pilot.RetentionInventory
		if json.Unmarshal(raw, &row) != nil {
			return nil, errors.New("maintenance receipt row invalid")
		}
		original = append(original, row)
	}
	adjusted, err := pilot.RetentionMaintenanceInventory(group, phase, original, until)
	if err != nil {
		return nil, err
	}
	output := []json.RawMessage{}
	for n, row := range adjusted {
		// Preserve every field of the original row. Only the exact retention expiry
		// can change; semantic comparison still rejects extra/missing/changed data.
		var fields map[string]json.RawMessage
		if json.Unmarshal(rows[n], &fields) != nil {
			return nil, errors.New("bad maintenance row")
		}
		fields["RetainUntil"], err = json.Marshal(row.RetainUntil)
		if err != nil {
			return nil, err
		}
		raw, e := json.Marshal(fields)
		if e != nil {
			return nil, e
		}
		output = append(output, raw)
	}
	return output, nil
}

func checkRetentionMaintenance(baseline, fresh, group, expectation, inventory string) (result, error) {
	var want retentionExpectation
	if _, err := strictPostFile(expectation, &want, 16); err != nil {
		return result{}, err
	}
	id, err := uuid.Parse(want.MaintenanceID)
	source, known := pilot.RemainingEnvironmentSources()[group]
	if err != nil || pilot.CheckRetentionMaintenanceHorizon(want.MaintenanceEnd, want.RetainUntil) != nil || want.ExpiresAt.After(want.MaintenanceEnd.Add(2*time.Hour)) || want.ExpiresAt.After(want.RetainUntil) || id == uuid.Nil || want.MaintenanceID == pilot.RemainingExecution || want.Audit == want.MaintenanceID ||
		(want.Phase != "before" && want.Phase != "after") || !hash.MatchString(want.BindingsSHA256) || want.Candidate != want.BindingsSHA256 ||
		!known || want.Core != source[0] || want.Native != source[1] || want.Network != source[2] || want.Frozen != source[3] {
		return result{}, errors.New("maintenance exact source/identity/phase differs")
	}
	var actual postV22Receipt
	if _, err := strictPostFile(filepath.Join(fresh, "retention-native-verification.json"), &actual, 20); err != nil {
		return result{}, err
	}
	if actual.MaintenanceID != want.MaintenanceID || actual.Phase != want.Phase || actual.BindingsSHA256 != want.BindingsSHA256 || actual.RetainUntil == nil || actual.MaintenanceEnd == nil || !actual.RetainUntil.Equal(want.RetainUntil) || !actual.MaintenanceEnd.Equal(want.MaintenanceEnd) {
		return result{}, errors.New("actual maintenance receipt identity/phase differs")
	}
	frozen, err := os.ReadFile(filepath.Join(baseline, "frozen-inventory.json"))
	if err != nil || digest(frozen) != want.Frozen {
		return result{}, errors.New("maintenance historical inventory anchor differs")
	}
	expected, err := readNativeContract(baseline, "expected-contract.json", "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION", "six-map.json")
	var original []json.RawMessage
	if err != nil || json.Unmarshal(frozen, &original) != nil || len(original) != len(expected.Copies) {
		return result{}, errors.New("maintenance historical membership differs")
	}
	for n := range original {
		if !semanticEqual(original[n], expected.Copies[n]) {
			return result{}, errors.New("maintenance historical protection differs")
		}
	}
	extra := 2 + len(pilot.RemainingRetainedIdentities(group))
	dormant, unassigned := pilot.RemainingDormantIdentity(group)
	if dormant != "" {
		value, present := expected.Users[dormant]
		if !present || value != "" || expected.Policies[unassigned] == "" {
			return result{}, errors.New("maintenance dormant identity differs")
		}
		extra++
	}
	if err = checkSixUncreated(fresh, group, want.Audit); err != nil {
		return result{}, err
	}
	return checkNativeAuditPhase(baseline, fresh, group, inventory, want.postV22Expectation,
		nativeAuditContract{true, "RETENTION_MAINTENANCE_NATIVE_READONLY", "retention-readonly.json", "retention-native-verification.json", want.BindingsSHA256, "PASS_COMPLETE_RETENTION_MAINTENANCE_NATIVE_AGGREGATION", extra, dormant}, want.Phase, want.RetainUntil)
}
