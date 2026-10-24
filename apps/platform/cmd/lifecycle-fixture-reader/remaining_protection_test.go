package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

func protectionFixture(t *testing.T) (domain.Copy, lc.CopyProtection, time.Time, time.Time) {
	t.Helper()
	scope, _ := pilot.RemainingScopeFor("mixed")
	var cp domain.Copy
	for _, row := range scope.Copies {
		if row.Key == "fixture/m1-c.csv" {
			cp = row
		}
	}
	if !remainingProtectedCopy(cp) {
		t.Fatal("fixed protected C missing")
	}
	at := time.Date(2026, 10, 23, 14, 0, 0, 0, time.UTC)
	until := time.Date(2026, 10, 24, 12, 40, 0, 0, time.UTC)
	return cp, lc.CopyProtection{Copy: cp, RetentionMode: "GOVERNANCE", RetainUntil: &until, ObservedAt: at}, at, until
}

func TestProtectedRemainingCommandSuccessAndCounterexamples(t *testing.T) {
	for _, name := range []string{"success", "expired", "equal-now", "short", "missing", "none", "compliance", "hold", "wrong-version", "wrong-provider", "wrong-expected-copy", "stale", "future", "unknown", "expired-horizon"} {
		t.Run(name, func(t *testing.T) {
			cp, p, at, until := protectionFixture(t)
			var inspectErr error
			switch name {
			case "expired":
				x := at.Add(-time.Second)
				p.RetainUntil = &x
			case "equal-now":
				p.RetainUntil = &at
			case "short":
				x := until.Add(-time.Second)
				p.RetainUntil = &x
			case "missing":
				p.RetainUntil = nil
			case "none":
				p.RetentionMode = "NONE"
			case "compliance":
				p.RetentionMode = "COMPLIANCE"
			case "hold":
				p.LegalHold = true
			case "wrong-version":
				p.Copy.ProviderVersion = uuid.NewString()
			case "wrong-provider":
				p.Copy.ProviderInstanceID = uuid.New()
			case "wrong-expected-copy":
				cp.ProviderVersion = uuid.NewString()
				p.Copy = cp
				if !remainingProtectedCopy(cp) {
					t.Fatal("known C identity bypassed guard")
				}
			case "stale":
				p.ObservedAt = at.Add(-time.Minute - time.Nanosecond)
			case "future":
				p.ObservedAt = at.Add(time.Nanosecond)
			case "unknown":
				inspectErr = errors.New("secret DSN should not be emitted")
			case "expired-horizon":
				until = at
			}
			commands := 0
			result, err := protectedRemainingCommand(context.Background(), cp, until,
				func(context.Context, domain.Copy) (lc.CopyProtection, error) { return p, inspectErr },
				func() (lc.DispatchResult, error) { commands++; return lc.DispatchResult{}, nil },
				func() time.Time { return at })
			if (err == nil) != (name == "success") || commands != map[bool]int{true: 1, false: 0}[name == "success"] {
				t.Fatalf("commands=%d err=%v", commands, err)
			}
			if result.Admitted || result.AttemptID != uuid.Nil {
				t.Fatal("non-delete case fabricated an attempt")
			}
		})
	}
}

func TestProtectedDispatchDiagnosticsDoNotConflateErrorAdmissionOrAttempt(t *testing.T) {
	for _, tc := range []struct {
		result lc.DispatchResult
		err    error
		reason string
	}{
		{lc.DispatchResult{}, nil, "REJECTED_WITHOUT_ATTEMPT"},
		{lc.DispatchResult{}, errors.New("password=DO_NOT_PUBLISH"), "DISPATCH_ERROR"},
		{lc.DispatchResult{Admitted: true}, nil, "UNEXPECTED_ADMISSION"},
		{lc.DispatchResult{AttemptID: uuid.New()}, nil, "UNEXPECTED_ATTEMPT"},
		{lc.DispatchResult{Admitted: true, AttemptID: uuid.New()}, errors.New("DO_NOT_PUBLISH"), "DISPATCH_ERROR"},
		{lc.DispatchResult{}, nativeProtectionError{"GOVERNANCE_RETENTION_EXPIRED"}, "DISPATCH_ERROR"},
	} {
		o := observeProtectedDispatch(tc.result, tc.err)
		raw, _ := json.Marshal(o)
		if o.Reason != tc.reason || o.Admitted != tc.result.Admitted || o.AttemptPresent != (tc.result.AttemptID != uuid.Nil) || o.DispatchError != (tc.err != nil) || strings.Contains(string(raw), "DO_NOT_PUBLISH") {
			t.Fatalf("incorrect non-secret observation: %s", raw)
		}
	}
}

type protectionJournal struct{ starts, observations int }

func (j *protectionJournal) StartStorageCall(context.Context, lc.StorageCallStart) error {
	j.starts++
	return nil
}
func (j *protectionJournal) ObserveStorageCall(context.Context, lc.StorageCallObservation) error {
	j.observations++
	return nil
}

// Actual existing SDK/Port HTTP decoding, with a local provider double. This is
// a protocol regression, not retained-source native acceptance.
func TestProtectedRemainingUsesActualInspectorProtocolWithoutNativeMutation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		cp, _, _, until := protectionFixture(t)
		at := time.Now().UTC()
		until = at.Add(time.Hour)
		expiry := until
		if expired {
			expiry = at.Add(-time.Second)
		}
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			requests++
			if req.Method != "GET" || req.URL.Path != "/"+cp.Namespace+"/"+cp.Key || req.URL.Query().Get("versionId") != cp.ProviderVersion {
				t.Error("expanded request")
			}
			w.Header().Set("Content-Type", "application/xml")
			if req.URL.Query().Has("retention") {
				fmt.Fprintf(w, "<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>%s</RetainUntilDate></Retention>", expiry.Format(time.RFC3339Nano))
			} else if req.URL.Query().Has("legal-hold") {
				fmt.Fprint(w, "<LegalHold><Status>OFF</Status></LegalHold>")
			} else {
				t.Error("unexpected query")
			}
		}))
		j := &protectionJournal{}
		inspector, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "local-test", SecretKey: "local-test", WorkspaceID: cp.WorkspaceID, ProviderInstanceID: cp.ProviderInstanceID, IncarnationID: cp.IncarnationID, Namespaces: namespaces, Journal: j})
		if err != nil {
			t.Fatal(err)
		}
		commands := 0
		_, err = protectedRemainingCommand(context.Background(), cp, until, inspector.InspectRetentionAndHold,
			func() (lc.DispatchResult, error) { commands++; return lc.DispatchResult{}, nil }, time.Now)
		server.Close()
		if (err != nil) != expired || requests != 2 || j.starts != 2 || j.observations != 2 || commands != map[bool]int{true: 0, false: 1}[expired] {
			t.Fatalf("expired=%v commands=%d requests=%d err=%v", expired, commands, requests, err)
		}
	}
}

func TestProtectionDeadlineUsesFixedWindowRatherThanShortRehearsal(t *testing.T) {
	end, _ := time.Parse(time.RFC3339, pilot.RemainingNotAfter)
	got, err := remainingProtectionDeadline(remainingConfig{CandidateUseUntil: end.Add(-time.Hour)})
	if err != nil || !got.Equal(end.Add(2*time.Hour)) {
		t.Fatal(got, err)
	}
	for _, deadline := range []time.Time{{}, end.Add(time.Second)} {
		if _, err := remainingProtectionDeadline(remainingConfig{CandidateUseUntil: deadline}); err == nil {
			t.Fatal("invalid window accepted")
		}
	}
}
