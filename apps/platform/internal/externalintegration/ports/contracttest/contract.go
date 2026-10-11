// Package contracttest is the shared conformance suite for publication adapters.
package contracttest

import (
	"context"
	"errors"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"testing"
)

type Factory func(*testing.T, string) ports.Adapter

func Run(t *testing.T, f Factory) {
	q := d.Request{WorkspaceID: uuid.New(), PublicationID: uuid.New(), ReleaseID: uuid.New(), BindingID: uuid.New(), IdentifierID: uuid.New(), Provider: "reference", Instance: "synthetic", AdapterRevision: "contract-v1", ExternalSubjectID: "subject", ExternalIdentifier: "release", RequestKey: "request/" + uuid.NewString()}
	for _, scenario := range []string{"published", "registered", "failed", "lost-response"} {
		t.Run(scenario, func(t *testing.T) {
			a := f(t, scenario)
			q := q
			q.AdapterRevision = a.Revision
			q.Provider = a.Provider
			q.Instance = a.Instance
			ctx := context.Background()
			dispatches := 0
			i := ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { dispatches++; return nil }}
			for kind, fn := range map[string]func(context.Context, d.Request, ports.Invocation) (d.ProviderResult, error){"IDENTITY": a.Identity.CheckIdentity, "IDENTIFIER": a.Identifier.CheckIdentifier} {
				r, err := fn(ctx, q, i)
				if err != nil || !r.Valid(q, kind) {
					t.Fatalf("%s receipt: %+v %v", kind, r, err)
				}
			}
			r, err := a.Catalog.Submit(ctx, q, i)
			if scenario == "lost-response" {
				if !errors.Is(err, d.ErrUnavailable) {
					t.Fatalf("lost response: %v", err)
				}
			} else if err != nil || !r.Valid(q, "SUBMIT") {
				t.Fatalf("submit: %+v %v", r, err)
			}
			polled, err := a.Catalog.Poll(ctx, q, i)
			if err != nil || !polled.Valid(q, "POLL") {
				t.Fatalf("poll: %+v %v", polled, err)
			}
			if scenario == "failed" {
				if r.State != "FAILED" || polled.State != "ABSENT" {
					t.Fatalf("failure contract: %+v %+v", r, polled)
				}
			} else {
				if polled.State != "PUBLISHED" {
					t.Fatalf("reconciliation: %+v", polled)
				}
				replay, err := a.Catalog.Submit(ctx, q, i)
				if err != nil || replay != polled {
					t.Fatalf("stable same-key catalog: %+v %v", replay, err)
				}
			}
			if dispatches < 4 {
				t.Fatalf("physical calls omitted dispatch: %d", dispatches)
			}
			sentinel := errors.New("dispatch rejected")
			called := 0
			_, err = a.Catalog.Submit(ctx, q, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { called++; return sentinel }})
			if !errors.Is(err, sentinel) || called != 1 {
				t.Fatalf("dispatch rejection: %v/%d", err, called)
			}
			before := dispatches
			err = a.Delivery.PrepareDelivery(ctx, q, i)
			if !errors.Is(err, d.ErrUnsupported) || dispatches != before {
				t.Fatal("unsupported delivery performed I/O")
			}
		})
	}
}
