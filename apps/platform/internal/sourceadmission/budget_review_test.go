package sourceadmission

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Both callers prepare the original operation before either response is
// consumed. Whichever rejection commits first owns the frozen error; a late
// contender must return that outcome after current authorization, not its own
// transient receipt/error. Neither outcome permits accepting corrupt bytes.
func TestPostgresBudgetConcurrentLateFailureReturnsFrozenOriginalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		corruptWins, revokeLate bool
	}{
		{name: "budget-wins"},
		{name: "integrity-wins", corruptWins: true},
		{name: "late-caller-revoked", revokeLate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			declareBundleTotal(t, f, MaxBytes+1)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var requests atomic.Int32
			f.sourceResponse = func(w http.ResponseWriter, r *http.Request, name string) bool {
				if requests.Add(1) == 2 {
					w.Header().Set("X-Test-Delayed-Corrupt", "true")
					_, _ = w.Write([]byte("{\"corrupt\":true}"))
					return true
				}
				return false
			}
			transport := f.s.Source.Client.Transport
			arrived := make(chan struct{}, 2)
			consumeValid, consumeCorrupt := make(chan struct{}), make(chan struct{})
			f.s.Source.Client.Transport = budgetRoundTripper(func(r *http.Request) (*http.Response, error) {
				response, e := transport.RoundTrip(r)
				arrived <- struct{}{}
				wait := consumeValid
				if response != nil && response.Header.Get("X-Test-Delayed-Corrupt") == "true" {
					wait = consumeCorrupt
				}
				select {
				case <-wait:
				case <-r.Context().Done():
				}
				return response, e
			})
			type result struct {
				receipt Receipt
				err     error
			}
			done := make(chan result, 2)
			for k := 0; k < 2; k++ {
				go func() { rec, e := f.s.Admit(ctx, f.r, f.caller); done <- result{rec, e} }()
			}
			for k := 0; k < 2; k++ {
				select {
				case <-arrived:
				case <-ctx.Done():
					t.Fatal("two physical calls did not arrive")
				}
			}
			winnerRelease, lateRelease := consumeValid, consumeCorrupt
			want, status := ErrUnsupported, http.StatusUnprocessableEntity
			if tc.corruptWins {
				winnerRelease, lateRelease = consumeCorrupt, consumeValid
				want, status = ErrIntegrity, http.StatusConflict
			}
			close(winnerRelease)
			winner := <-done
			if !errors.Is(winner.err, want) || winner.receipt.State != "FAILED_FINAL" {
				t.Fatalf("terminal winner: %+v %v", winner.receipt, winner.err)
			}
			if tc.revokeLate {
				f.allowed.Store(false)
			}
			close(lateRelease)
			late := <-done
			if tc.revokeLate {
				if !errors.Is(late.err, ErrDenied) {
					t.Errorf("frozen result bypassed current authority: %v", late.err)
				}
				status = http.StatusForbidden
			} else if !errors.Is(late.err, want) || late.receipt.State != "FAILED_FINAL" || late.receipt.ErrorCode != want.Error() || late.receipt.ID != winner.receipt.ID {
				t.Errorf("late result diverged from frozen original key: %+v %v", late.receipt, late.err)
			}
			w := httptest.NewRecorder()
			respond(w, late.receipt, late.err)
			if w.Code != status {
				t.Errorf("late HTTP=%d want=%d", w.Code, status)
			}
			f.allowed.Store(true)
			saved, e := f.s.LookupKey(ctx, f.r.WorkspaceID, f.r.Key, f.caller)
			if e != nil || saved.ID != winner.receipt.ID || saved.State != "FAILED_FINAL" || saved.ErrorCode != want.Error() {
				t.Fatalf("frozen lookup: %+v %v", saved, e)
			}
			if replay, e := f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, want) || replay.ID != saved.ID || replay.State != "FAILED_FINAL" || replay.ErrorCode != saved.ErrorCode {
				t.Fatalf("terminal replay drifted: %+v %v", replay, e)
			}
			assertBudgetCalls(t, f, 2, "RECEIVED", 200)
			assertBudgetRejectionFacts(t, f, winner.receipt.ID, 1)
			assertNoBudgetPublication(t, f)
		})
	}
}
