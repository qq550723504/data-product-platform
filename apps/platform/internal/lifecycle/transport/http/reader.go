package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

// A closed fixture-only route. No production processing/delivery routes,
// locators, credentials, tenant selection or stopped-I/O receipt ingestion.
type ReaderHandler struct {
	Service     FixtureReader
	Resolver    principal.Resolver
	WorkspaceID uuid.UUID
}

type FixtureReader interface {
	ReadTo(context.Context, lc.AdmitFixtureReaderCommand, io.Writer) (lc.FixtureReadResult, error)
}

func (h ReaderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/fixture/read" {
		http.NotFound(w, r)
		return
	}
	if h.Service == nil || h.Resolver == nil || h.WorkspaceID == uuid.Nil {
		http.Error(w, "fixture reader unavailable", 503)
		return
	}
	caller, e := h.Resolver.Resolve(r, h.WorkspaceID, lc.CapabilityAdmitReader)
	if e != nil {
		http.Error(w, "fixture reader denied", 403)
		return
	}
	var body struct {
		CopyID   uuid.UUID `json:"copy_id"`
		ReaderID uuid.UUID `json:"reader_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || body.CopyID == uuid.Nil || body.ReaderID == uuid.Nil {
		http.Error(w, "invalid fixture read", 400)
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		http.Error(w, "invalid fixture read", 400)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	tracked := &responseWriter{ResponseWriter: w, ctx: r.Context()}
	result, e := h.Service.ReadTo(lc.WithFixtureReaderCaller(r.Context(), caller), lc.AdmitFixtureReaderCommand{CommandHeader: lc.CommandHeader{WorkspaceID: h.WorkspaceID, IdempotencyKey: r.Header.Get("Idempotency-Key")}, CopyID: body.CopyID, ReaderID: body.ReaderID}, tracked)
	if tracked.written > 0 {
		return
	} // an interrupted transfer never becomes a new payload/error stream
	if e != nil {
		http.Error(w, "fixture read blocked or stop unconfirmed", 403)
		return
	}
	if result.Admission.Replayed {
		http.Error(w, "new read attempt required", 409)
		return
	}
}

type responseWriter struct {
	http.ResponseWriter
	ctx     context.Context
	written int64
}

func (w *responseWriter) Write(b []byte) (int, error) {
	return w.WriteContext(w.ctx, b)
}
func (w *responseWriter) WriteContext(ctx context.Context, b []byte) (int, error) {
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	controller := http.NewResponseController(w.ResponseWriter)
	if until, ok := ctx.Deadline(); ok {
		if e := controller.SetWriteDeadline(until); e != nil {
			return 0, e
		}
	}
	n, e := w.ResponseWriter.Write(b)
	w.written += int64(n)
	if e == nil && n > 0 {
		e = controller.Flush()
	}
	return n, e
}
