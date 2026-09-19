package outbox

import (
	"fmt"
	"strings"
)

// Route declares the required handler obligations for one event type.
type Route struct {
	// EventType is the outbox_event.event_type this route applies to.
	EventType string
	// RequiredHandlers lists the handler names that must confirm before the
	// event may be marked PUBLISHED. Declaration order is the confirmation
	// order and is stable across retries.
	//
	// An empty list is an explicit retention-only declaration: the event has
	// no external side effect owed by any consumer. Retention-only events must
	// still be declared here; an event type absent from the routing table is an
	// error and is never completed implicitly.
	RequiredHandlers []string
}

// Router is the deterministic routing contract between event types and the
// handlers that must confirm them. It is versioned: completing an event is a
// property of the routing version that declared it, not of whichever handlers
// happen to be registered at dispatch time.
type Router struct {
	version string
	routes  map[string][]string
	order   []string
}

// NewRouter validates and freezes a routing declaration. The same version must
// always describe the same required-handler sets, so a version that changes
// obligations is a new version reviewed as a deliberate change.
func NewRouter(version string, routes []Route) (*Router, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("outbox routing version is required")
	}
	r := &Router{version: version, routes: make(map[string][]string, len(routes))}
	for _, route := range routes {
		eventType := strings.TrimSpace(route.EventType)
		if eventType == "" {
			return nil, fmt.Errorf("outbox routing version %s has a route with an empty event type", version)
		}
		if _, exists := r.routes[eventType]; exists {
			return nil, fmt.Errorf("outbox routing version %s declares event type %q more than once", version, eventType)
		}
		seen := make(map[string]struct{}, len(route.RequiredHandlers))
		required := make([]string, 0, len(route.RequiredHandlers))
		for _, raw := range route.RequiredHandlers {
			name := strings.TrimSpace(raw)
			if name == "" {
				return nil, fmt.Errorf("outbox route %q has an empty required handler", eventType)
			}
			if _, dup := seen[name]; dup {
				return nil, fmt.Errorf("outbox route %q requires handler %q more than once", eventType, name)
			}
			seen[name] = struct{}{}
			required = append(required, name)
		}
		r.routes[eventType] = required
		r.order = append(r.order, eventType)
	}
	return r, nil
}

// Version identifies this routing contract in dispatch logs and diagnostics.
func (r *Router) Version() string { return r.version }

// RequiredHandlers returns the required handlers for an event type. ok is false
// when the event type is not declared in this routing version; callers must
// treat that as an error and must not mark the event complete.
//
// The result is always a copy and is never nil: an explicit retention-only
// route returns a non-nil empty slice so callers can tell it apart from
// "not routed" via ok.
func (r *Router) RequiredHandlers(eventType string) ([]string, bool) {
	required, ok := r.routes[eventType]
	if !ok {
		return nil, false
	}
	copied := make([]string, len(required))
	copy(copied, required)
	return copied, true
}

// Obligation implements the ObligationSource / obligationResolver contract used
// to freeze an event's required handlers when it is recorded or first claimed.
func (r *Router) Obligation(eventType string) (string, []string, bool) {
	required, ok := r.RequiredHandlers(eventType)
	if !ok {
		return "", nil, false
	}
	return r.version, required, true
}

// EventTypes returns the declared event types in declaration order.
func (r *Router) EventTypes() []string {
	return append([]string(nil), r.order...)
}

// validate fails when a route requires a handler with no registered
// implementation. The dispatcher refuses to start instead of silently
// completing events whose handlers are missing, and never records a false
// success confirmation for a handler it did not run.
func (r *Router) validate(registered map[string]Handler) error {
	missing := make([]string, 0)
	seen := make(map[string]struct{})
	for _, eventType := range r.order {
		for _, name := range r.routes[eventType] {
			if _, ok := registered[name]; ok {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"outbox routing version %s requires handlers that are not registered: %s",
			r.version, strings.Join(missing, ", "),
		)
	}
	return nil
}
