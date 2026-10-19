package main

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"net/url"
	"os"
	"path/filepath"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

type sixExpectation struct {
	postV22Expectation
	Execution, Profile, Role string
}

// Create an explicit expectation from the immutable eighteen-copy baseline and
// the current generated policies. It is never labelled an actual observation.
func exportSixExpected(baseline, output, group, inventory, frozenPath string) error {
	old, err := readReceipt(baseline)
	if err != nil {
		return err
	}
	frozen, err := os.ReadFile(frozenPath)
	if err != nil || digest(frozen) != pilot.RemainingEnvironmentSources()[group][3] || old.Group != group {
		return errors.New("known historical inventory required")
	}
	var historical []json.RawMessage
	if json.Unmarshal(frozen, &historical) != nil || len(historical) != len(old.Copies) {
		return errors.New("historical receipt membership differs")
	}
	for n := range historical {
		if !semanticEqual(historical[n], old.Copies[n]) {
			return errors.New("historical receipt identities or protection differ")
		}
	}
	var original, present []struct{ Copy domain.Copy }
	current, err := os.ReadFile(inventory)
	if err != nil || json.Unmarshal(frozen, &original) != nil || json.Unmarshal(current, &present) != nil {
		return errors.New("exact inventory required")
	}
	var filtered []domain.Copy
	var cp []domain.Copy
	for _, row := range original {
		if row.Copy.Key != "fixture/p1.csv" {
			filtered = append(filtered, row.Copy)
		}
	}
	for _, row := range present {
		cp = append(cp, row.Copy)
	}
	if len(filtered) != len(cp) {
		return errors.New("present inventory membership differs")
	}
	for n := range cp {
		if cp[n] != filtered[n] {
			return errors.New("present inventory identity differs")
		}
	}
	if err = policyFiles(baseline, old.Policies); err != nil {
		return err
	}
	users, policies, err := pilot.RemainingAuditPolicies(group, cp)
	if err != nil {
		return err
	}
	files := map[string][]byte{"frozen-inventory.json": frozen}
	for name := range old.Policies {
		b, e := os.ReadFile(filepath.Join(baseline, "native-policy-"+name+".json"))
		if e != nil {
			return e
		}
		files["native-policy-"+name+".json"] = b
	}
	for user, name := range users {
		if _, exists := old.Users[user]; exists {
			return errors.New("new user already in historical receipt")
		}
		old.Users[user] = name
	}
	for name, b := range policies {
		old.Policies[name] = digest(b)
		files["native-policy-"+name+".json"] = b
	}
	old.Status = "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION"
	old.Source = "six-map.json"
	b, err := json.Marshal(old)
	if err != nil {
		return err
	}
	files["expected-contract.json"] = append(b, '\n')
	if err = os.Mkdir(output, 0700); err != nil {
		return err
	}
	for name, b := range files {
		f, e := os.OpenFile(filepath.Join(output, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func checkSix(baseline, fresh, group, expectation, inventory, mapping string) (result, error) {
	var want sixExpectation
	if _, err := strictPostFile(expectation, &want, 14); err != nil {
		return result{}, err
	}
	raw, err := os.ReadFile(mapping)
	if err != nil {
		return result{}, err
	}
	var m pilot.RemainingEnvironmentMap
	if _, err = strictPostFile(mapping, &m, 3); err != nil {
		return result{}, err
	}
	if want.Execution != pilot.RemainingExecution || want.Profile != pilot.RemainingProfile || want.Candidate != digest(raw) ||
		!m.Matches(group, want.Role, [3]string{want.Core, want.Native, want.Network}) {
		return result{}, errors.New("six exact current environment/role differs")
	}
	// Historical eighteen-copy membership remains an expectation, with its
	// original exact bytes bound independently of the seventeen present copies.
	frozen, err := os.ReadFile(filepath.Join(baseline, "frozen-inventory.json"))
	if err != nil || digest(frozen) != want.Frozen || want.Frozen != pilot.RemainingEnvironmentSources()[group][3] {
		return result{}, errors.New("six historical inventory anchor differs")
	}
	var expected receipt
	expected, err = readNativeContract(baseline, "expected-contract.json", "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION", "six-map.json")
	if err != nil {
		return result{}, err
	}
	var copies []json.RawMessage
	if json.Unmarshal(frozen, &copies) != nil || len(copies) != len(expected.Copies) {
		return result{}, errors.New("six historical membership differs")
	}
	for n, cp := range copies {
		if !semanticEqual(cp, expected.Copies[n]) {
			return result{}, errors.New("six expected historical identity/protection differs")
		}
	}
	extra := 2 + len(pilot.RemainingRetainedIdentities(group))
	dormant, unassigned := pilot.RemainingDormantIdentity(group)
	if dormant != "" {
		value, present := expected.Users[dormant]
		if !present || value != "" || expected.Policies[unassigned] == "" {
			return result{}, errors.New("retained partial identity or unassigned policy differs")
		}
		extra++
	}
	if err = checkSixUncreated(fresh, group, want.Audit); err != nil {
		return result{}, err
	}
	return checkNativeAudit(baseline, fresh, group, inventory, want.postV22Expectation,
		nativeAuditContract{true, "SIX_TARGET_PREPARATION_NATIVE_READONLY", "six-readonly.json", "six-native-verification.json", digest(raw), "PASS_COMPLETE_SIX_NATIVE_AGGREGATION", extra, dormant})
}

func checkSixUncreated(fresh, group, audit string) error {
	var proof struct {
		Audit string `json:"audit_id"`
		Users map[string]struct {
			Attempt string `json:"attempt_id"`
			Status  int    `json:"status"`
			Code    string `json:"code"`
		} `json:"users"`
	}
	if _, err := strictPostFile(filepath.Join(fresh, "six-uncreated-users.json"), &proof, 2); err != nil {
		return err
	}
	users := pilot.RemainingUncreatedIdentities(group)
	if proof.Audit != audit || len(proof.Users) != len(users) {
		return errors.New("uncreated identity proof scope differs")
	}
	seen := map[string]bool{}
	for _, user := range users {
		row, ok := proof.Users[user]
		id, err := uuid.Parse(row.Attempt)
		if !ok || err != nil || id == uuid.Nil || row.Attempt != id.String() || seen[row.Attempt] || row.Status != 404 || row.Code != "XMinioAdminNoSuchUser" {
			return errors.New("uncreated identity absence is not exact")
		}
		seen[row.Attempt] = true
		var start struct {
			ID, Method, Path, Query string
			Audit                   string `json:"audit_id"`
			At                      any
		}
		var observation struct {
			ID      string
			Status  int
			Error   bool   `json:"transport_error"`
			Audit   string `json:"audit_id"`
			At      any
			Code    string `json:"provider_error_code"`
			Request string `json:"provider_request_id"`
		}
		if _, err = strictPostFile(filepath.Join(fresh, "readonly-start-"+row.Attempt+".json"), &start, 6); err != nil {
			return err
		}
		if _, err = strictPostFile(filepath.Join(fresh, "readonly-observation-"+row.Attempt+".json"), &observation, 7); err != nil {
			return err
		}
		q, err := url.ParseQuery(start.Query)
		if err != nil || len(q) != 1 || len(q["accessKey"]) != 1 || q.Get("accessKey") != user || start.ID != row.Attempt || start.Audit != audit || start.Method != "GET" || start.Path != "/minio/admin/v3/user-info" || observation.ID != row.Attempt || observation.Audit != audit || observation.Status != 404 || observation.Error {
			return errors.New("uncreated identity journal differs")
		}
	}
	return nil
}
