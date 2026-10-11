package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	lifecycle "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

type deleteJournal struct {
	claimed         bool
	proof           domain.ProviderObservation
	failObservation bool
}

func (j *deleteJournal) ClaimExactSend(_ context.Context, _ domain.ProviderAttempt, _ domain.Copy) error {
	if j.claimed {
		return lifecycle.ErrIdempotencyConflict
	}
	j.claimed = true
	return nil
}
func (j *deleteJournal) ObserveExactSend(_ context.Context, p domain.ProviderObservation) error {
	j.proof = p
	if j.failObservation {
		return errors.New("lost database")
	}
	return nil
}
func deleteFixture() (domain.Copy, domain.ProviderAttempt) {
	c := inspectionFixture()
	a := domain.ProviderAttempt{ID: uuid.New(), WorkspaceID: c.WorkspaceID, CopyID: c.ID, DispatchID: uuid.New(), PlanID: uuid.New(), Action: domain.DeleteExact, StartedAt: time.Now().UTC().Add(-time.Minute), DependencyRevision: 1}
	a.ProviderRequestKey = "lifecycle-delete:" + a.ID.String()
	return c, a
}
func TestExactDeleterNoHiddenRetryNoReplayNoBypass(t *testing.T) {
	for _, status := range []int{204, 403, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c, a := deleteFixture()
			j := &deleteJournal{}
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "DELETE" || r.URL.Query().Get("versionId") != c.ProviderVersion || len(r.URL.Query()) != 1 || r.Header.Get("X-Amz-Bypass-Governance-Retention") != "" {
					t.Error("expanded deletion")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			deleter, e := NewLifecycleDeleter(LifecycleDeleteConfig{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "fixture-executor", SecretKey: "fixture-secret", Targets: []domain.Copy{c}, Journal: j})
			if e != nil {
				t.Fatal(e)
			}
			e = deleter.DeleteExactCopy(context.Background(), a, c)
			if (status == 204) != (e == nil) {
				t.Fatalf("status=%d err=%v", status, e)
			}
			if calls.Load() != 1 || !j.claimed || j.proof.Certainty != domain.InvocationConfirmed {
				t.Fatal("retry or missing invocation proof")
			}
			if e = deleter.DeleteExactCopy(context.Background(), a, c); e == nil || calls.Load() != 1 {
				t.Fatal("same attempt sent twice")
			}
			foreign := c
			foreign.ProviderVersion = "foreign-version"
			if e = deleter.DeleteExactCopy(context.Background(), a, foreign); e == nil || calls.Load() != 1 {
				t.Fatal("unapproved version sent")
			}
		})
	}
}
func TestExactDeleterLostResponseIsUnknownAndNeverResent(t *testing.T) {
	c, a := deleteFixture()
	j := &deleteJournal{}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, e := w.(http.Hijacker).Hijack()
		if e == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	d, e := NewLifecycleDeleter(LifecycleDeleteConfig{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "fixture-executor", SecretKey: "fixture-secret", Targets: []domain.Copy{c}, Journal: j})
	if e != nil {
		t.Fatal(e)
	}
	if e = d.DeleteExactCopy(context.Background(), a, c); e == nil || calls.Load() != 1 || j.proof.Outcome != domain.ProviderUnknown || j.proof.InvokedAt == nil {
		t.Fatalf("lost response erased physical fact: %+v %v", j.proof, e)
	}
	if e = d.DeleteExactCopy(context.Background(), a, c); e == nil || calls.Load() != 1 {
		t.Fatal("blind resend")
	}
}
