// Package ports contains provider-neutral contracts, with no vendor SDK dependencies.
package ports

import (
	"context"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"sort"
)

// Dispatch MUST commit before external I/O. One invocation is one physical call;
// adapters may not retry, redirect, spawn detached work, or perform I/O after
// returning. A marker conflict must stop I/O. Context cancellation is respected.
type Invocation struct {
	ID       uuid.UUID
	Dispatch func(context.Context) error
}
type ExternalIdentityProvider interface {
	CheckIdentity(context.Context, d.Request, Invocation) (d.ProviderResult, error)
}
type ExternalIdentifierProvider interface {
	CheckIdentifier(context.Context, d.Request, Invocation) (d.ProviderResult, error)
}
type CatalogPublisher interface {
	Submit(context.Context, d.Request, Invocation) (d.ProviderResult, error)
	Poll(context.Context, d.Request, Invocation) (d.ProviderResult, error)
}
type DeliveryConnector interface {
	PrepareDelivery(context.Context, d.Request, Invocation) error
}

// Read is replayable metadata lookup, never acknowledgement/deletion/delivery.
// It obeys Invocation's dispatch/cancellation/no-hidden-retry contract.
type UsageEventSource interface {
	Read(context.Context, d.UsageReadRequest, Invocation) (d.UsagePage, error)
}
type Capabilities struct {
	IdentityVerification   bool `json:"identityVerification"`
	IdentifierVerification bool `json:"identifierVerification"`
	CatalogPublication     bool `json:"catalogPublication"`
	StableRequestKeys      bool `json:"stableRequestKeys"`
	AuthoritativeLookup    bool `json:"authoritativeLookup"`
	DataDelivery           bool `json:"dataDelivery"`
	UsageEvents            bool `json:"usageEvents"`
}
type Descriptor struct {
	Provider     string       `json:"provider"`
	Instance     string       `json:"instance"`
	Revision     string       `json:"revision"`
	Capabilities Capabilities `json:"capabilities"`
}
type Adapter struct {
	Descriptor
	Identity   ExternalIdentityProvider
	Identifier ExternalIdentifierProvider
	Catalog    CatalogPublisher
	Delivery   DeliveryConnector
	Usage      UsageEventSource
}
type Registry struct{ entries map[string]Adapter }

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{entries: map[string]Adapter{}}
	for _, a := range adapters {
		c := a.Capabilities
		if c.UsageEvents != (a.Usage != nil) {
			return nil, d.ErrUnsupported
		}
		if !d.ValidProvider(a.Provider) || !d.ValidText(a.Instance, 128) || !d.ValidText(a.Revision, 64) || a.Identity == nil || a.Identifier == nil || a.Catalog == nil || a.Delivery == nil || !c.IdentityVerification || !c.IdentifierVerification || !c.CatalogPublication || !c.StableRequestKeys || !c.AuthoritativeLookup || c.DataDelivery {
			return nil, d.ErrUnsupported
		}
		if _, ok := r.entries[a.Provider]; ok {
			return nil, d.ErrConflict
		}
		r.entries[a.Provider] = a
	}
	return r, nil
}
func (r *Registry) Get(provider string) (Adapter, error) {
	if r != nil {
		if a, ok := r.entries[provider]; ok {
			return a, nil
		}
	}
	return Adapter{}, d.ErrUnsupported
}
func (r *Registry) List() []Descriptor {
	out := []Descriptor{}
	if r != nil {
		for _, a := range r.entries {
			out = append(out, a.Descriptor)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}
