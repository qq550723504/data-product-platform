package contracttest

import (
	"context"
	"errors"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"testing"
)

func RunUsage(t *testing.T, f func(*testing.T) (ports.UsageEventSource, func() int64)) {
	source, count := f(t)
	q := d.UsageReadRequest{Source: d.UsageSource{ID: uuid.New(), WorkspaceID: uuid.New(), Provider: "reference", Instance: "synthetic", Stream: "urn:test:usage", AdapterRevision: "v1"}, Limit: d.UsagePageLimit}
	ctx := context.Background()
	markers := 0
	invocation := ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { markers++; return nil }}
	first, err := source.Read(ctx, q, invocation)
	if err != nil {
		t.Fatal(err)
	}
	first, err = first.Normalize(q)
	if err != nil {
		t.Fatal(err)
	}
	second, err := source.Read(ctx, q, invocation)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 1 || len(second.Events) != 1 || first.Events[0].Hash() != second.Events[0].Hash() || first.NextCursor != second.NextCursor || markers != 2 || count() != 2 {
		t.Fatal("read replay/context/physical calls changed")
	}
	stop := errors.New("dispatch denied")
	_, err = source.Read(ctx, q, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { return stop }})
	if !errors.Is(err, stop) || count() != 2 {
		t.Fatal("rejected dispatch performed I/O")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = source.Read(canceled, q, invocation)
	if err == nil || count() != 2 {
		t.Fatal("canceled read performed I/O")
	}
}
