package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

type returnTestJournal struct {
	claimErr, observationErr error
	claims, observations     int
	proof                    domain.ProviderObservation
}

func (j *returnTestJournal) ClaimExactSend(context.Context, domain.ProviderAttempt, domain.Copy) error {
	j.claims++
	return j.claimErr
}
func (j *returnTestJournal) ObserveExactSend(_ context.Context, p domain.ProviderObservation) error {
	j.observations++
	j.proof = p
	return j.observationErr
}

func returnAttempt(t *testing.T, group, stage string) (domain.Copy, domain.ProviderAttempt) {
	t.Helper()
	cp, err := fixedTarget(group, stage)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.ProviderAttempt{ID: uuid.New(), WorkspaceID: cp.WorkspaceID, CopyID: cp.ID,
		DispatchID: uuid.New(), PlanID: uuid.New(), Action: domain.DeleteExact,
		StartedAt: time.Now().UTC().Add(-time.Second), DependencyRevision: 1}
	a.ProviderRequestKey = "lifecycle-delete:" + a.ID.String()
	return cp, a
}

func testDeleter(t *testing.T, endpoint string, cp domain.Copy, j *journal) *storage.LifecycleDeleter {
	t.Helper()
	d, err := storage.NewLifecycleDeleter(storage.LifecycleDeleteConfig{Endpoint: strings.TrimPrefix(endpoint, "http://"),
		AccessKey: "test-delete", SecretKey: "test-only", Targets: []domain.Copy{cp}, Journal: j})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func readReturn(t *testing.T, p string) sendReturn {
	t.Helper()
	var r sendReturn
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &r) != nil {
		t.Fatal("missing structured return", err)
	}
	return r
}

func TestClaimRejectionKeepsOriginalErrorWithoutProviderObservationOrCost(t *testing.T) {
	for _, at := range []string{"authority", "budget", "Core"} {
		t.Run(at, func(t *testing.T) {
			cp, a := returnAttempt(t, "business-races", "P2")
			original := errors.New("specific " + at + " rejection")
			exact := &returnTestJournal{}
			budget := t.TempDir()
			check := func() error { return nil }
			if at == "authority" {
				check = func() error { return original }
			}
			if at == "Core" {
				exact.claimErr = original
			}
			if at == "budget" {
				if err := os.WriteFile(filepath.Join(budget, cp.ID.String()+".json"), []byte("retained slot"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) }))
			defer provider.Close()
			j := &journal{exact: exact, authorize: check, budget: budget}
			sendErr := testDeleter(t, provider.URL, cp, j).DeleteExactCopy(context.Background(), a, cp)
			if sendErr == nil || j.claimError == nil {
				t.Fatal("claim rejection was not retained")
			}
			path := filepath.Join(t.TempDir(), "returned.json")
			lookupCalled := false
			err := finishSendReturn(path, "P2", a, cp, j, sendErr, func() (string, string, error) {
				lookupCalled = true
				return "", "", pgx.ErrNoRows
			})
			if err == nil || lookupCalled || calls.Load() != 0 || exact.observations != 0 {
				t.Fatal("claim failure queried an observation, invoked provider or created a proof", err)
			}
			if at != "budget" && !errors.Is(err, original) {
				t.Fatal("original failure was masked", err)
			}
			if at == "budget" && !errors.Is(err, os.ErrExist) {
				t.Fatal("durable slot conflict was masked", err)
			}
			r := readReturn(t, path)
			if r.FailurePhase != "PRE_SEND_CLAIM_REJECTED" || r.ProviderInvocation != "NOT_ATTEMPTED" || r.ProviderObservationPresent || r.Certainty != "" || r.Outcome != "" || r.TransportObservation != nil || r.ClaimConfirmed || r.AutomaticResend || r.AutomaticRecovery || r.BudgetConsumed != (at == "Core") {
				t.Fatalf("invented invocation/NOT_SENT proof or wrong budget facts: %+v", r)
			}
		})
	}
}

func TestAcceptedProviderThenLostObservationKeepsTransportAndBothErrors(t *testing.T) {
	cp, a := returnAttempt(t, "business-races", "P2")
	storeErr := errors.New("original observation persistence failure")
	exact := &returnTestJournal{observationErr: storeErr}
	j := &journal{exact: exact, authorize: func() error { return nil }, budget: t.TempDir()}
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer provider.Close()
	sendErr := testDeleter(t, provider.URL, cp, j).DeleteExactCopy(context.Background(), a, cp)
	path := filepath.Join(t.TempDir(), "returned.json")
	err := finishSendReturn(path, "P2", a, cp, j, sendErr, func() (string, string, error) { return "", "", pgx.ErrNoRows })
	if !errors.Is(err, storeErr) || !errors.Is(err, pgx.ErrNoRows) || calls.Load() != 1 || exact.observations != 1 {
		t.Fatal("lost physical observation or original persistence error", err)
	}
	r := readReturn(t, path)
	if r.ProviderObservationPresent || r.ProviderInvocation != domain.InvocationConfirmed || r.TransportObservation == nil || r.TransportObservation.InvokedAt == nil || r.TransportObservation.Outcome != domain.ProviderAccepted || !r.BudgetConsumed || !r.ClaimConfirmed {
		t.Fatalf("accepted physical call was hidden by DB failure: %+v", r)
	}
}

func TestExpectedResponseLossPreservesConfirmedUnknownAndCannotResend(t *testing.T) {
	cp, a := returnAttempt(t, "late-hold", "H2")
	exact := &returnTestJournal{}
	budget := t.TempDir()
	j := &journal{exact: exact, authorize: func() error { return nil }, budget: budget}
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer provider.Close()
	d := testDeleter(t, provider.URL, cp, j)
	sendErr := d.DeleteExactCopy(context.Background(), a, cp)
	path := filepath.Join(t.TempDir(), "returned.json")
	if err := finishSendReturn(path, "H2", a, cp, j, sendErr, func() (string, string, error) { return exact.proof.Certainty, exact.proof.Outcome, nil }); err != nil {
		t.Fatal(err)
	}
	r := readReturn(t, path)
	if calls.Load() != 1 || r.Certainty != domain.InvocationConfirmed || r.Outcome != domain.ProviderUnknown || !r.SendError || r.AutomaticResend || r.AutomaticRecovery {
		t.Fatalf("response loss rewritten: %+v", r)
	}
	fresh := &journal{exact: exact, authorize: func() error { return nil }, budget: budget}
	if err := testDeleter(t, provider.URL, cp, fresh).DeleteExactCopy(context.Background(), a, cp); err == nil || calls.Load() != 1 || exact.claims != 1 {
		t.Fatal("new sender instance bypassed the retained budget")
	}
}

func TestObservationWriteErrorSurvivesSuccessfulReadbackForEverySendOutcome(t *testing.T) {
	for _, tc := range []struct{ group, stage string }{{"business-races", "P2"}, {"late-hold", "H2"}, {"mixed", "M1B"}} {
		t.Run(tc.stage, func(t *testing.T) {
			cp, a := returnAttempt(t, tc.group, tc.stage)
			original := errors.New("observation commit result uncertain")
			exact := &returnTestJournal{observationErr: original}
			j := &journal{exact: exact, authorize: func() error { return nil }, budget: t.TempDir()}
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if tc.stage == "P2" {
					w.WriteHeader(204)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
			}))
			defer provider.Close()
			sendErr := testDeleter(t, provider.URL, cp, j).DeleteExactCopy(context.Background(), a, cp)
			path := filepath.Join(t.TempDir(), "returned.json")
			err := finishSendReturn(path, tc.stage, a, cp, j, sendErr, func() (string, string, error) {
				return exact.proof.Certainty, exact.proof.Outcome, nil
			})
			if !errors.Is(err, original) || calls.Load() != 1 {
				t.Fatal("readback erased original write error", err)
			}
			r := readReturn(t, path)
			if r.FailurePhase != "PROVIDER_OBSERVATION_PERSISTENCE_FAILED" || !r.SendError || !r.ProviderObservationPresent || r.TransportObservation == nil || r.TransportObservation.InvokedAt == nil || r.TransportObservation.Outcome != exact.proof.Outcome || r.ProviderInvocation != domain.InvocationConfirmed || r.AutomaticRecovery || r.AutomaticResend {
				t.Fatalf("transport/error facts lost after readback: %+v", r)
			}
			if err = finishSendReturn(path, tc.stage, a, cp, j, sendErr, func() (string, string, error) { return exact.proof.Certainty, exact.proof.Outcome, nil }); !errors.Is(err, original) || !errors.Is(err, os.ErrExist) {
				t.Fatal("receipt collision erased persistence failure", err)
			}
		})
	}
}

func TestPreSendRejectionPreservesOExclusiveReceiptAndOriginalCause(t *testing.T) {
	cp, a := returnAttempt(t, "business-races", "P2")
	path := filepath.Join(t.TempDir(), "returned.json")
	if err := writePreSendRejection(path, "P2", a, cp, "PRIOR_ABSENCE_PENDING", lc.ErrStorageDenied); !errors.Is(err, lc.ErrStorageDenied) {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(path)
	if err := writePreSendRejection(path, "P2", a, cp, "PRIOR_ABSENCE_PENDING", lc.ErrStorageDenied); !errors.Is(err, os.ErrExist) || !errors.Is(err, lc.ErrStorageDenied) {
		t.Fatal("receipt collision masked rejection", err)
	}
	after, _ := os.ReadFile(path)
	if string(original) != string(after) {
		t.Fatal("original rejection overwritten")
	}
}
