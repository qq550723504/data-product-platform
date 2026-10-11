package testkit

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

type UsageSource struct {
	mu    sync.Mutex
	fn    func(context.Context, d.UsageReadRequest) (d.UsagePage, error)
	Reads atomic.Int64
}

func (s *UsageSource) Set(fn func(context.Context, d.UsageReadRequest) (d.UsagePage, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fn = fn
}
func UsagePage(q d.UsageReadRequest, events ...d.UsageOccurrence) d.UsagePage {
	return d.UsagePage{Instance: q.Source.Instance, RequestHash: q.Hash(), FromCursor: q.Cursor, NextCursor: q.Cursor + "n", Events: append([]d.UsageOccurrence{}, events...)}
}
func (s *UsageSource) Read(ctx context.Context, q d.UsageReadRequest, i ports.Invocation) (d.UsagePage, error) {
	if i.ID == uuid.Nil || i.Dispatch == nil {
		return d.UsagePage{}, d.ErrInvalid
	}
	if ctx.Err() != nil {
		return d.UsagePage{}, ctx.Err()
	}
	if err := i.Dispatch(ctx); err != nil {
		return d.UsagePage{}, err
	}
	s.Reads.Add(1)
	s.mu.Lock()
	fn := s.fn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, q)
	}
	return UsagePage(q, d.UsageOccurrence{ID: "example", Source: q.Source.Stream, Category: "USAGE", OccurredAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)}), nil
}
func (s *UsageSource) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var q d.UsageReadRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if r.Method != "POST" || r.URL.Path != "/usage/read" || json.NewDecoder(r.Body).Decode(&q) != nil {
		w.WriteHeader(400)
		return
	}
	p, err := s.Read(r.Context(), q, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { return nil }})
	if err != nil {
		w.WriteHeader(503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}
func (s *UsageSource) Server() *httptest.Server { return httptest.NewServer(s) }
