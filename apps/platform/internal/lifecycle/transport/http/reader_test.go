package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type recordingReader struct {
	calls  int
	cmd    lc.AdmitFixtureReaderCommand
	replay bool
}

type flushOwner struct {
	header           http.Header
	entered, release chan struct{}
	deadline         time.Time
	writes           int
}

func (w *flushOwner) Header() http.Header                { return w.header }
func (w *flushOwner) WriteHeader(int)                    {}
func (w *flushOwner) Write(b []byte) (int, error)        { w.writes++; return len(b), nil }
func (w *flushOwner) SetWriteDeadline(t time.Time) error { w.deadline = t; return nil }
func (w *flushOwner) FlushError() error                  { close(w.entered); <-w.release; return io.ErrClosedPipe }
func TestHTTPPinnedOutputIncludesFlushAndDeadline(t *testing.T) {
	owner := &flushOwner{header: http.Header{}, entered: make(chan struct{}), release: make(chan struct{})}
	until := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), until)
	defer cancel()
	w := &responseWriter{ResponseWriter: owner, ctx: ctx}
	done := make(chan error, 1)
	go func() { _, e := w.WriteContext(ctx, []byte("synthetic")); done <- e }()
	<-owner.entered
	if owner.writes != 1 || !owner.deadline.Equal(until) {
		t.Fatal("owned write deadline missing")
	}
	select {
	case <-done:
		t.Fatal("write returned before actual flush stopped")
	default:
	}
	cancel()
	select {
	case <-done:
		t.Fatal("cancel treated as flush stop")
	default:
	}
	close(owner.release)
	if e := <-done; !errors.Is(e, io.ErrClosedPipe) {
		t.Fatal(e)
	}
	unsupported := &responseWriter{ResponseWriter: httptest.NewRecorder(), ctx: ctx}
	if n, e := unsupported.WriteContext(ctx, []byte("synthetic")); n != 0 || !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled output emitted bytes")
	}
	fresh, c := context.WithDeadline(context.Background(), until)
	defer c()
	if n, e := unsupported.WriteContext(fresh, []byte("synthetic")); n != 0 || !errors.Is(e, http.ErrNotSupported) {
		t.Fatal("unsupported deadline wrote bytes", n, e)
	}
}

func (s *recordingReader) ReadTo(_ context.Context, c lc.AdmitFixtureReaderCommand, w io.Writer) (lc.FixtureReadResult, error) {
	s.calls++
	s.cmd = c
	if s.replay {
		return lc.FixtureReadResult{Admission: lc.ReaderAdmission{Replayed: true}}, nil
	}
	n, e := w.Write([]byte("synthetic\n"))
	return lc.FixtureReadResult{BytesWritten: int64(n)}, e
}
func TestClosedReaderHTTPRejectsUntrustedInputsBeforeRead(t *testing.T) {
	w, actor, copy, reader := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	resolver, e := principal.NewStaticResolver(true, "test-token", "server-owned", actor.String(), []string{w.String()}, []string{lc.CapabilityAdmitReader})
	if e != nil {
		t.Fatal(e)
	}
	valid := `{"copy_id":"` + copy.String() + `","reader_id":"` + reader.String() + `"}`
	for _, tc := range []struct {
		name, path, token, body string
		status                  int
	}{
		{"missing-auth", "/fixture/read", "", valid, 403},
		{"wrong-auth", "/fixture/read", "other", valid, 403},
		{"legacy-storage", "/storage/download", "test-token", valid, 404},
		{"production-delivery", "/api/v1/datasets/deliver", "test-token", valid, 404},
		{"tenant-field", "/fixture/read", "test-token", strings.TrimSuffix(valid, "}") + `,"workspace_id":"` + uuid.New().String() + `"}`, 400},
		{"locator-field", "/fixture/read", "test-token", strings.TrimSuffix(valid, "}") + `,"uri":"s3://foreign/key"}`, 400},
		{"fake-stop", "/fixture/read", "test-token", strings.TrimSuffix(valid, "}") + `,"stopped_at":"2026-01-01T00:00:00Z"}`, 400},
		{"trailing", "/fixture/read", "test-token", valid + ` {}`, 400},
		{"oversized", "/fixture/read", "test-token", valid + strings.Repeat(" ", 17<<10), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &recordingReader{}
			h := ReaderHandler{service, resolver, w}
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			r.Header.Set("X-Actor-ID", uuid.New().String())
			r.Header.Set("Idempotency-Key", "read-1")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != tc.status || service.calls != 0 || strings.Contains(rr.Body.String(), "synthetic") {
				t.Fatalf("status %d calls %d", rr.Code, service.calls)
			}
		})
	}
	service := &recordingReader{}
	h := ReaderHandler{service, resolver, w}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/fixture/read", strings.NewReader(valid))
		r.Header.Set("Authorization", "Bearer test-token")
		r.Header.Set("Idempotency-Key", "read-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if i == 0 {
			if rr.Code != 200 || rr.Body.String() != "synthetic\n" || service.cmd.WorkspaceID != w || service.cmd.CopyID != copy || service.cmd.ReaderID != reader {
				t.Fatal("trusted command/payload mismatch")
			}
			service.replay = true
		} else if rr.Code != 409 || strings.Contains(rr.Body.String(), "synthetic") {
			t.Fatal("replay exposed payload")
		}
	}
}
