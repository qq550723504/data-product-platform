package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func postProfile(t *testing.T, group string) (postV22ReadProfile, []byte, []byte) {
	t.Helper()
	frozen, e := os.ReadFile(filepath.Join("testdata/post-v22", group+".json"))
	if e != nil {
		t.Fatal(e)
	}
	var full []remainingExpected
	if json.Unmarshal(frozen, &full) != nil {
		t.Fatal("inventory")
	}
	present := []remainingExpected{}
	for _, o := range full {
		if o.Copy.ID.String() != absentP1 {
			present = append(present, o)
		}
	}
	raw, _ := json.Marshal(present)
	source := postV22Sources[group]
	return postV22ReadProfile{Enabled: true, AuditID: uuid.New(), Purpose: postV22Purpose, Group: group, CandidateSHA256: postV22Candidate,
		BinarySHA256: strings.Repeat("b", 64), Core: source[0], Native: source[1], Network: source[2], Endpoint: "127.0.0.1:9000",
		RootAccess: "synthetic", RootSecret: "synthetic", FrozenInventorySHA256: source[3], PresentInventorySHA256: postV22Hash(raw),
		NotBefore: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(10 * time.Minute), MaxProviderDeletes: 0}, frozen, raw
}
func TestPostV22ExplicitPresentInventoriesAllGroups(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		t.Run(group, func(t *testing.T) {
			p, full, present := postProfile(t, group)
			raw, _ := json.Marshal(p)
			if _, e := parsePostV22Profile(raw, time.Now().UTC()); e != nil {
				t.Fatal(e)
			}
			original, actual, absent, e := postV22Inventories(p, full, present)
			if e != nil || len(actual) != map[string]int{"business-races": 7, "late-hold": 6, "mixed": 4}[group] || (group == "business-races") != (absent != nil) {
				t.Fatalf("inventory %v", e)
			}
			if len(original) != len(actual)+boolForTest(absent != nil) {
				t.Fatal("policy scope changed")
			}
		})
	}
}
func boolForTest(v bool) int {
	if v {
		return 1
	}
	return 0
}
func TestPostV22RejectsAuthorityAndCredentialExpansion(t *testing.T) {
	cases := map[string]func(map[string]any){
		"missing-zero":    func(d map[string]any) { delete(d, "MaxProviderDeletes") },
		"delete":          func(d map[string]any) { d["MaxProviderDeletes"] = 1 },
		"DB":              func(d map[string]any) { d["PostgresDSN"] = "postgres://synthetic" },
		"business-token":  func(d map[string]any) { d["Token"] = "synthetic" },
		"producer-secret": func(d map[string]any) { d["ProducerSecret"] = "synthetic" },
		"old-execution":   func(d map[string]any) { d["AuditID"] = "8e41cfba-a41e-44dd-9ce8-d6365b3bb90e" },
		"foreign-core":    func(d map[string]any) { d["Core"] = strings.Repeat("b", 64) },
		"foreign-native":  func(d map[string]any) { d["Native"] = strings.Repeat("b", 64) },
		"foreign-network": func(d map[string]any) { d["Network"] = strings.Repeat("b", 64) },
		"candidate":       func(d map[string]any) { d["CandidateSHA256"] = strings.Repeat("b", 64) },
		"endpoint":        func(d map[string]any) { d["Endpoint"] = "foreign:9000" },
		"expired":         func(d map[string]any) { d["ExpiresAt"] = time.Now().UTC().Add(-time.Second) },
		"future":          func(d map[string]any) { d["NotBefore"] = time.Now().UTC().Add(time.Minute) },
		"long-window":     func(d map[string]any) { d["ExpiresAt"] = time.Now().UTC().Add(time.Hour) },
		"disabled":        func(d map[string]any) { d["Enabled"] = false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p, _, _ := postProfile(t, "business-races")
			raw, _ := json.Marshal(p)
			var d map[string]any
			_ = json.Unmarshal(raw, &d)
			change(d)
			raw, _ = json.Marshal(d)
			if _, e := parsePostV22Profile(raw, time.Now().UTC()); e == nil {
				t.Fatal("expanded or inconsistent profile accepted")
			}
		})
	}
}
func TestPostV22RejectsOmissionSubstitutionAndProtectionDrift(t *testing.T) {
	for _, name := range []string{"P1-present", "different-missing", "extra", "version", "hold", "retention", "bytes", "sha", "frozen-bytes"} {
		t.Run(name, func(t *testing.T) {
			p, full, present := postProfile(t, "business-races")
			var rows []remainingExpected
			_ = json.Unmarshal(present, &rows)
			switch name {
			case "P1-present":
				present = full
			case "different-missing":
				var all []remainingExpected
				_ = json.Unmarshal(full, &all)
				rows = all[1:]
			case "extra":
				rows = append(rows, rows[0])
			case "version":
				rows[0].Copy.ProviderVersion = "foreign"
			case "hold":
				rows[0].LegalHold = !rows[0].LegalHold
			case "retention":
				rows[0].Retention = "GOVERNANCE"
			case "bytes":
				rows[0].Copy.ByteSize++
			case "sha":
				rows[0].Copy.ContentSHA256 = strings.Repeat("a", 64)
			case "frozen-bytes":
				full = append(full, ' ')
			}
			if name != "P1-present" {
				present, _ = json.Marshal(rows)
			}
			p.PresentInventorySHA256 = postV22Hash(present)
			if _, _, _, e := postV22Inventories(p, full, present); e == nil {
				t.Fatal("inventory drift accepted")
			}
		})
	}
}

type postRoundTripper func(*http.Request) (*http.Response, error)

func (f postRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostV22SDKAbsenceAndBoundTransport(t *testing.T) {
	p, full, present := postProfile(t, "business-races")
	_, _, cp, e := postV22Inventories(p, full, present)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		status        int
		code, request string
		allowed       bool
	}{
		{404, "NoSuchVersion", "mock-provider-request", true}, {404, "NoSuchKey", "mock-provider-request", true},
		{404, "", "", false}, {404, "NoSuchVersion", "", false}, {404, "NotFound", "mock-provider-request", false},
		{200, "", "", false}, {403, "AccessDenied", "mock-provider-request", false}, {500, "InternalError", "mock-provider-request", false},
	} {
		t.Run(http.StatusText(tc.status)+tc.code+tc.request, func(t *testing.T) {
			c := config{Endpoint: "127.0.0.1:9000", RootAccess: "mock", RootSecret: "mock", OutputDirectory: t.TempDir()}
			calls := 0
			transport := &postV22VerifyTransport{c: c, p1: cp, base: postRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				header := make(http.Header)
				header.Set("X-Minio-Error-Code", tc.code)
				header.Set("X-Amz-Request-Id", tc.request)
				return &http.Response{StatusCode: tc.status, Header: header, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			})}
			got, e := postV22Absence(context.Background(), c, cp, transport)
			if tc.allowed != (e == nil) || (e == nil && got == nil) || calls != 1 {
				t.Fatalf("status%d calls%d %v", tc.status, calls, e)
			}
			starts, _ := filepath.Glob(filepath.Join(c.OutputDirectory, "readonly-start-*.json"))
			observations, _ := filepath.Glob(filepath.Join(c.OutputDirectory, "readonly-observation-*.json"))
			if len(starts) != 1 || len(observations) != 1 {
				t.Fatal("missing original request channels")
			}
		})
	}
	for _, method := range []string{"DELETE", "POST", "PUT", "PATCH"} {
		calls := 0
		tr := &postV22VerifyTransport{c: config{Endpoint: p.Endpoint}, base: postRoundTripper(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }), p1: &domain.Copy{}}
		req, _ := http.NewRequest(method, "http://"+p.Endpoint+"/bucket/key", nil)
		if _, e := tr.RoundTrip(req); e == nil || calls != 0 {
			t.Fatal("mutation dispatched")
		}
	}
}
