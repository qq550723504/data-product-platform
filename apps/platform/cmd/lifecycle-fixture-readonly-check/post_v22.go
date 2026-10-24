package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
)

const postV22Schema = "POST_V22_SEVENTEEN_NATIVE_READONLY"
const postV22Candidate = "f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243"
const p1Copy = "5a4c9789-3e67-4e54-ab58-128d111cbd65"

type postV22Expectation struct {
	Audit     string    `json:"audit_id"`
	Profile   string    `json:"profile_sha256"`
	Binary    string    `json:"binary_sha256"`
	Candidate string    `json:"candidate_sha256"`
	Frozen    string    `json:"frozen_inventory_sha256"`
	Present   string    `json:"present_inventory_sha256"`
	Core      string    `json:"core"`
	Native    string    `json:"native"`
	Network   string    `json:"network"`
	NotBefore time.Time `json:"not_before"`
	ExpiresAt time.Time `json:"expires_at"`
}
type postV22Receipt struct {
	MaintenanceID  string    `json:"maintenance_id,omitempty"`
	Phase          string    `json:"phase,omitempty"`
	BindingsSHA256 string    `json:"bindings_sha256,omitempty"`
	Schema         string    `json:"schema"`
	Audit          string    `json:"audit_id"`
	Profile        string    `json:"profile_sha256"`
	Binary         string    `json:"binary_sha256"`
	Candidate      string    `json:"candidate_sha256"`
	Frozen         string    `json:"frozen_inventory_sha256"`
	Core           string    `json:"core"`
	Native         string    `json:"native"`
	Network        string    `json:"network"`
	NotBefore      time.Time `json:"not_before"`
	ExpiresAt      time.Time `json:"expires_at"`
	Started        time.Time `json:"started_at"`
	Finished       time.Time `json:"finished_at"`
	Absence        *struct {
		Copy    json.RawMessage `json:"copy"`
		Attempt string          `json:"attempt_id"`
		Status  int             `json:"status"`
		Code    string          `json:"code"`
	} `json:"p1_absence"`
	Verification receipt `json:"verification"`
}

func strictPostFile(path string, out any, members int) ([]byte, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return nil, e
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return nil, errors.New("trailing post-v22 file data")
	}
	if members > 0 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields) != members {
			return nil, errors.New("missing post-v22 fields")
		}
	}
	return raw, nil
}
func semanticEqual(a, b []byte) bool {
	var x, y any
	dx, dy := json.NewDecoder(bytes.NewReader(a)), json.NewDecoder(bytes.NewReader(b))
	dx.UseNumber()
	dy.UseNumber()
	return dx.Decode(&x) == nil && dy.Decode(&y) == nil && reflect.DeepEqual(x, y)
}

type nativeAuditContract struct {
	expectedBaseline                                   bool
	schema, sourceFile, receiptFile, candidate, status string
	extraIdentities                                    int
	dormant                                            string
}

func checkPostV22(baseline, fresh, group, expectation, inventory string) (result, error) {
	var want postV22Expectation
	if _, err := strictPostFile(expectation, &want, 11); err != nil {
		return result{Group: group}, err
	}
	return checkNativeAudit(baseline, fresh, group, inventory, want, nativeAuditContract{false, postV22Schema, "post-v22-readonly.json", "post-v22-native-verification.json", postV22Candidate, "PASS_COMPLETE_POST_V22_NATIVE_AGGREGATION", 0, ""})
}

func checkNativeAudit(baseline, fresh, group, inventory string, want postV22Expectation, contract nativeAuditContract) (result, error) {
	return checkNativeAuditPhase(baseline, fresh, group, inventory, want, contract, "")
}

func checkNativeAuditPhase(baseline, fresh, group, inventory string, want postV22Expectation, contract nativeAuditContract, phase string) (result, error) {
	out := result{Group: group}
	count, ok := map[string][2]int{"business-races": {7, 4}, "late-hold": {6, 2}, "mixed": {4, 2}}[group]
	if !ok {
		return out, errors.New("unsupported native audit group")
	}
	count[1] += contract.extraIdentities
	var a receipt
	var e error
	if contract.expectedBaseline {
		a, e = readNativeContract(baseline, "expected-contract.json", "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION", "six-map.json")
	} else {
		a, e = readReceipt(baseline)
	}
	if e != nil {
		return out, e
	}
	audit, e := uuid.Parse(want.Audit)
	if e != nil || audit == uuid.Nil || want.Audit == "8e41cfba-a41e-44dd-9ce8-d6365b3bb90e" || want.Candidate != contract.candidate ||
		!hash.MatchString(want.Profile) || !hash.MatchString(want.Binary) || !hash.MatchString(want.Frozen) || !hash.MatchString(want.Present) ||
		!hash.MatchString(want.Core) || !hash.MatchString(want.Native) || !hash.MatchString(want.Network) ||
		want.NotBefore.IsZero() || want.ExpiresAt.Sub(want.NotBefore) <= 0 || want.ExpiresAt.Sub(want.NotBefore) > 55*time.Minute {
		return out, errors.New("post-v22 expectation scope or lifetime differs")
	}
	var r postV22Receipt
	members := 15
	if phase != "" {
		members = 18
	}
	raw, e := strictPostFile(filepath.Join(fresh, contract.receiptFile), &r, members)
	if e != nil {
		return out, e
	}
	// Nested explicit safety fields are mandatory too; zero-value decoding alone is insufficient.
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(root["verification"], &fields)
	b := r.Verification
	if len(fields) != 9 || string(fields["delete"]) != "0" || string(fields["mutations"]) != "0" ||
		b.Source != contract.sourceFile || b.Status != "PASS_EXACT_NATIVE_READONLY" || r.Schema != contract.schema ||
		r.Audit != want.Audit || r.Profile != want.Profile || r.Binary != want.Binary || r.Candidate != want.Candidate || r.Frozen != want.Frozen ||
		r.Core != want.Core || r.Native != want.Native || r.Network != want.Network || !r.NotBefore.Equal(want.NotBefore) || !r.ExpiresAt.Equal(want.ExpiresAt) ||
		r.Started.Before(want.NotBefore) || r.Finished.Before(r.Started) || !r.Finished.Before(want.ExpiresAt) ||
		a.Inventory != want.Frozen || b.Inventory != want.Present || len(b.Copies) != count[0] {
		return out, errors.New("post-v22 receipt binding or explicit safety fields differ")
	}
	present, e := os.ReadFile(inventory)
	if e != nil || digest(present) != want.Present {
		return out, errors.New("present inventory bytes differ")
	}
	var supplied []json.RawMessage
	if json.Unmarshal(present, &supplied) != nil || len(supplied) != count[0] {
		return out, errors.New("present inventory incomplete")
	}
	var p1 json.RawMessage
	retained := []json.RawMessage{}
	for _, item := range a.Copies {
		var entry struct{ Copy struct{ ID string } }
		if json.Unmarshal(item, &entry) != nil {
			return out, errors.New("bad baseline copy")
		}
		if entry.Copy.ID == p1Copy {
			if group != "business-races" || p1 != nil {
				return out, errors.New("unexpected P1 scope")
			}
			var row map[string]json.RawMessage
			_ = json.Unmarshal(item, &row)
			p1 = row["Copy"]
		} else {
			retained = append(retained, item)
		}
	}
	if len(retained) != count[0] || len(a.Copies) != count[0]+boolInt(group == "business-races") || (group == "business-races") != (p1 != nil) {
		return out, errors.New("baseline must have exactly one permitted absence")
	}
	if phase != "" {
		retained, e = retentionPhaseRows(group, phase, retained)
		if e != nil {
			return out, e
		}
	}
	for i := range retained {
		if !semanticEqual(retained[i], supplied[i]) || !semanticEqual(retained[i], b.Copies[i]) {
			return out, errors.New("exact present content/version/protection differs")
		}
	}
	if e = checkPostV22Journals(fresh, r); e != nil {
		return out, e
	}
	if group == "business-races" {
		if e = checkP1Journal(fresh, r, p1); e != nil {
			return out, e
		}
	} else if r.Absence != nil {
		return out, errors.New("absence outside business scope")
	}
	// This compares actual present receipts; it never fabricates a present P1.
	// Original IAM policy membership remains independent of present membership.
	a.Copies = retained
	out, e = compareNativeReceiptsWithDormant(baseline, fresh, group, a, b, count[1], contract.dormant)
	if e != nil {
		return out, e
	}
	out.Status = contract.status
	return out, nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func checkP1Journal(fresh string, r postV22Receipt, p1 json.RawMessage) error {
	if r.Absence == nil || !semanticEqual(r.Absence.Copy, p1) || r.Absence.Status != 404 || (r.Absence.Code != "NoSuchVersion" && r.Absence.Code != "NoSuchKey") {
		return errors.New("missing exact official P1 absence")
	}
	id, e := uuid.Parse(r.Absence.Attempt)
	if e != nil || id == uuid.Nil {
		return errors.New("invalid P1 attempt identity")
	}
	var cp struct{ Namespace, Key, ProviderVersion string }
	_ = json.Unmarshal(p1, &cp)
	var start struct {
		ID, Method, Path, Query string
		Audit                   string `json:"audit_id"`
		At                      time.Time
	}
	var observation struct {
		ID                string
		Status            int
		TransportError    bool `json:"transport_error"`
		At                time.Time
		ProviderCode      string `json:"provider_error_code"`
		ProviderRequestID string `json:"provider_request_id"`
		Audit             string `json:"audit_id"`
	}
	if _, e = strictPostFile(filepath.Join(fresh, "readonly-start-"+id.String()+".json"), &start, 6); e != nil {
		return e
	}
	if _, e = strictPostFile(filepath.Join(fresh, "readonly-observation-"+id.String()+".json"), &observation, 7); e != nil {
		return e
	}
	query, e := url.ParseQuery(start.Query)
	if e != nil || len(query) != 1 || len(query["versionId"]) != 1 || query.Get("versionId") != cp.ProviderVersion || start.ID != id.String() || observation.ID != id.String() || start.Audit != r.Audit || observation.Audit != r.Audit ||
		start.Method != "HEAD" || start.Path != "/"+cp.Namespace+"/"+cp.Key || observation.Status != 404 || observation.TransportError || observation.ProviderCode != r.Absence.Code || observation.ProviderRequestID == "" ||
		start.At.Before(r.Started) || observation.At.Before(start.At) || observation.At.After(r.Finished) {
		return errors.New("P1 exact version HEAD observation differs")
	}
	return nil
}

func checkPostV22Journals(fresh string, r postV22Receipt) error {
	starts, e := filepath.Glob(filepath.Join(fresh, "readonly-start-*.json"))
	if e != nil {
		return e
	}
	observations, e := filepath.Glob(filepath.Join(fresh, "readonly-observation-*.json"))
	if e != nil {
		return e
	}
	if len(starts) == 0 || len(starts) != len(observations) {
		return errors.New("post-v22 request journal membership incomplete")
	}
	ids := map[string]bool{}
	for _, path := range starts {
		var s struct {
			ID, Method, Path, Query string
			Audit                   string `json:"audit_id"`
			At                      time.Time
		}
		if _, e = strictPostFile(path, &s, 6); e != nil {
			return e
		}
		id, e := uuid.Parse(s.ID)
		if e != nil || id == uuid.Nil || ids[s.ID] || filepath.Base(path) != "readonly-start-"+id.String()+".json" || s.Audit != r.Audit ||
			(s.Method != "GET" && s.Method != "HEAD") || !strings.HasPrefix(s.Path, "/") || s.At.Before(r.Started) || s.At.After(r.Finished) {
			return errors.New("old, foreign or unsafe request start")
		}
		ids[s.ID] = true
		var o struct {
			ID      string
			Audit   string `json:"audit_id"`
			Status  int
			Error   bool   `json:"transport_error"`
			Code    string `json:"provider_error_code"`
			Request string `json:"provider_request_id"`
			At      time.Time
		}
		if _, e = strictPostFile(filepath.Join(fresh, "readonly-observation-"+s.ID+".json"), &o, 7); e != nil {
			return e
		}
		if o.ID != s.ID || o.Audit != r.Audit || o.Error || o.Status < 100 || o.Status > 599 || o.At.Before(s.At) || o.At.After(r.Finished) {
			return errors.New("old, foreign or incomplete request observation")
		}
	}
	for _, path := range observations {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "readonly-observation-"), ".json")
		if !ids[id] {
			return errors.New("orphan observation")
		}
	}
	return nil
}
