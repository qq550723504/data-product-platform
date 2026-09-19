package outbox

import (
	"context"
	"strings"
	"testing"
)

func TestNewRouterRejectsInvalidDeclarations(t *testing.T) {
	cases := []struct {
		name    string
		version string
		routes  []Route
		want    string
	}{
		{"missing version", "  ", []Route{{EventType: "A"}}, "routing version is required"},
		{"empty event type", "v1", []Route{{EventType: " "}}, "empty event type"},
		{"duplicate event type", "v1", []Route{{EventType: "A"}, {EventType: "A"}}, "more than once"},
		{"empty handler name", "v1", []Route{{EventType: "A", RequiredHandlers: []string{" "}}}, "empty required handler"},
		{"duplicate handler", "v1", []Route{{EventType: "A", RequiredHandlers: []string{"h", "h"}}}, "more than once"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRouter(tc.version, tc.routes)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewRouter error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestRouterRequiredHandlersAreOrderedAndCopied(t *testing.T) {
	router, err := NewRouter("v1", []Route{
		{EventType: "TwoHandlers", RequiredHandlers: []string{"second", "first"}},
		{EventType: "RetentionOnly"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	required, ok := router.RequiredHandlers("TwoHandlers")
	if !ok {
		t.Fatal("TwoHandlers must be routed")
	}
	if len(required) != 2 || required[0] != "second" || required[1] != "first" {
		t.Fatalf("required handlers = %v, want declaration order [second first]", required)
	}
	required[0] = "mutated"
	again, _ := router.RequiredHandlers("TwoHandlers")
	if again[0] != "second" {
		t.Fatal("RequiredHandlers must return a copy; router state was mutated")
	}

	retention, ok := router.RequiredHandlers("RetentionOnly")
	if !ok || len(retention) != 0 {
		t.Fatalf("retention-only route = %v ok=%v, want explicit empty declaration", retention, ok)
	}
}

func TestRouterTreatsUnknownEventTypeAsUnrouted(t *testing.T) {
	router, err := NewRouter("v1", []Route{{EventType: "Known"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if _, ok := router.RequiredHandlers("Unknown"); ok {
		t.Fatal("an undeclared event type must not be routed implicitly")
	}
}

func TestRouterValidateRejectsMissingHandler(t *testing.T) {
	router, err := NewRouter("v1", []Route{
		{EventType: "A", RequiredHandlers: []string{"present"}},
		{EventType: "B", RequiredHandlers: []string{"absent"}},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	registered := map[string]Handler{"present": func(_ context.Context, _ PublishedEvent) error { return nil }}
	err = router.validate(registered)
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("validate error = %v, want the missing handler name", err)
	}

	registered["absent"] = func(_ context.Context, _ PublishedEvent) error { return nil }
	if err := router.validate(registered); err != nil {
		t.Fatalf("validate with all handlers = %v, want nil", err)
	}
}
