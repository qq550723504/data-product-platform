// Package testkit supplies a deterministic synthetic provider for shared adapter
// contracts and PostgreSQL fault tests. It is never registered by composition roots.
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
)

type Provider struct {
	Mu                                       sync.Mutex
	Counts                                   map[string]int
	Remote                                   map[string]d.ProviderResult
	SubmitState, PollState, PreflightFailure string
	LostResponse, Mismatch                   bool
	Hook                                     func(string, d.Request, ports.Invocation)
}

func New() *Provider {
	return &Provider{Counts: map[string]int{}, Remote: map[string]d.ProviderResult{}, SubmitState: "PUBLISHED"}
}
func (p *Provider) Count(kind string) int { p.Mu.Lock(); defer p.Mu.Unlock(); return p.Counts[kind] }
func (p *Provider) Facts() int            { p.Mu.Lock(); defer p.Mu.Unlock(); return len(p.Remote) }
func Result(q d.Request, state string) d.ProviderResult {
	return d.ProviderResult{Instance: q.Instance, RequestKey: q.RequestKey, RequestHash: q.Hash(), State: state}
}
func (p *Provider) invoke(ctx context.Context, q d.Request, i ports.Invocation, kind string) (d.ProviderResult, error) {
	if ctx.Err() != nil {
		return d.ProviderResult{}, ctx.Err()
	}
	if i.ID == uuid.Nil || i.Dispatch == nil {
		return d.ProviderResult{}, d.ErrInvalid
	}
	if err := i.Dispatch(ctx); err != nil {
		return d.ProviderResult{}, err
	}
	p.Mu.Lock()
	p.Counts[kind]++
	hook := p.Hook
	p.Mu.Unlock()
	if hook != nil {
		hook(kind, q, i)
	}
	p.Mu.Lock()
	defer p.Mu.Unlock()
	r := Result(q, "VERIFIED")
	switch kind {
	case "IDENTITY":
		r.ExternalID = q.ExternalSubjectID
	case "IDENTIFIER":
		r.ExternalID = q.ExternalIdentifier
	case "SUBMIT":
		if old, ok := p.Remote[q.RequestKey]; ok {
			return old, nil
		}
		r = Result(q, p.SubmitState)
		if r.State == "PUBLISHED" || r.State == "REGISTERED" {
			r.ExternalID = q.ExternalIdentifier
			r.RemoteID = "catalog:" + q.PublicationID.String()
			p.Remote[q.RequestKey] = r
		}
		if p.LostResponse {
			return d.ProviderResult{}, d.ErrUnavailable
		}
	case "POLL":
		r = Result(q, "ABSENT")
		if old, ok := p.Remote[q.RequestKey]; ok {
			r = old
			if p.PollState != "" {
				r.State = p.PollState
				if r.State == "ABSENT" || r.State == "FAILED" {
					r.ExternalID = ""
					r.RemoteID = ""
					delete(p.Remote, q.RequestKey)
				} else {
					p.Remote[q.RequestKey] = r
				}
			}
		}
	}
	if p.PreflightFailure == kind {
		r = Result(q, "FAILED")
	}
	if p.Mismatch && (kind == "SUBMIT" || kind == "POLL") {
		r.ExternalID = "wrong-resource"
	}
	return r, nil
}
func (p *Provider) CheckIdentity(c context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return p.invoke(c, q, i, "IDENTITY")
}
func (p *Provider) CheckIdentifier(c context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return p.invoke(c, q, i, "IDENTIFIER")
}
func (p *Provider) Submit(c context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return p.invoke(c, q, i, "SUBMIT")
}
func (p *Provider) Poll(c context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return p.invoke(c, q, i, "POLL")
}
func (p *Provider) PrepareDelivery(context.Context, d.Request, ports.Invocation) error {
	return d.ErrUnsupported
}
func (p *Provider) Adapter(provider, instance string) ports.Adapter {
	return ports.Adapter{Descriptor: ports.Descriptor{Provider: provider, Instance: instance, Revision: "fake-v1", Capabilities: ports.Capabilities{IdentityVerification: true, IdentifierVerification: true, CatalogPublication: true, StableRequestKeys: true, AuthoritativeLookup: true}}, Identity: p, Identifier: p, Catalog: p, Delivery: p}
}
func (p *Provider) Server() *httptest.Server {
	return p.ServerWithUsage(nil)
}
func (p *Provider) ServerWithUsage(source *UsageSource) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/usage/read" && source != nil {
			source.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var body struct {
			Request d.Request `json:"request"`
			Hash    string    `json:"requestHash"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Hash != body.Request.Hash() {
			w.WriteHeader(400)
			return
		}
		kind := map[string]string{"/identity/check": "IDENTITY", "/identifier/check": "IDENTIFIER", "/catalog/submit": "SUBMIT", "/catalog/poll": "POLL"}[r.URL.Path]
		if kind == "" {
			w.WriteHeader(404)
			return
		}
		out, err := p.invoke(r.Context(), body.Request, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { return nil }}, kind)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
}
