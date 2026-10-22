package domain

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestUsageIdentityNormalizationAndPageFence(t *testing.T) {
	q := UsageReadRequest{Source: UsageSource{ID: uuid.New(), Stream: "urn:fixture:events", Instance: "fixture"}, Limit: UsagePageLimit}
	v := UsageOccurrence{ID: "event", Source: q.Source.Stream, Category: "USAGE", OccurredAt: time.Date(2026, 10, 11, 8, 0, 0, 123456789, time.FixedZone("SG", 8*3600))}
	utc := v
	utc.OccurredAt = v.OccurredAt.UTC().Truncate(time.Microsecond)
	if v.Hash() != utc.Hash() {
		t.Fatal("same occurrence changes identity with timezone/precision")
	}
	p := UsagePage{Instance: q.Source.Instance, RequestHash: q.Hash(), NextCursor: "next", Events: []UsageOccurrence{v}}
	if _, err := p.Normalize(q); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []UsagePage{{Instance: "other"}, {Instance: q.Source.Instance, RequestHash: q.Hash(), NextCursor: "", Events: p.Events}, {Instance: q.Source.Instance, RequestHash: q.Hash(), NextCursor: "https://secret", Events: p.Events}} {
		if _, err := bad.Normalize(q); err == nil {
			t.Fatal("unbound/non-progressing page accepted")
		}
	}
	p.Events[0].Category = "FUTURE"
	if _, err := p.Normalize(q); err != nil || p.Events[0].KnownCategory() {
		t.Fatal("unknown category must reach quarantine")
	}
	if ValidStream("https://secret/path") || ValidStream("urn:bad\nstream") || ValidCursor("token?a=b") {
		t.Fatal("URLs/control characters admitted")
	}
}
