package oidc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type fixture struct {
	Now    int64          `json:"now"`
	Claims map[string]any `json:"claims"`
	Cases  []struct {
		Name       string         `json:"name"`
		Accept     bool           `json:"accept"`
		AcceptCore bool           `json:"acceptCore"`
		Set        map[string]any `json:"set"`
		Remove     []string       `json:"remove"`
	} `json:"cases"`
}

func contract(t *testing.T) fixture {
	t.Helper()
	data, err := os.ReadFile("../../../../../../docs/contracts/oidc-introspection-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

type bindingFunc func(context.Context, Identity, uuid.UUID) (Binding, error)

func (f bindingFunc) Lookup(ctx context.Context, i Identity, w uuid.UUID) (Binding, error) {
	return f(ctx, i, w)
}

var workspace = uuid.MustParse("10000000-0000-0000-0000-000000000001")
var actor = uuid.MustParse("20000000-0000-0000-0000-000000000001")

func activeBinding() Binding {
	return Binding{Active: true, Subject: "platform:person-001", ActorID: actor, WorkspaceID: workspace,
		Capabilities: map[string]struct{}{principal.CapabilityHumanDecision: {}}}
}

func newTestResolver(t *testing.T, handler http.HandlerFunc, bindings Bindings) *Resolver {
	t.Helper()
	s := httptest.NewTLSServer(handler)
	t.Cleanup(s.Close)
	r, err := NewResolver(Config{Issuer: "https://idp.example.test", TokenEndpoint: s.URL + "/token",
		IntrospectionEndpoint: s.URL + "/introspect", ClientID: "resource-client", ClientSecret: "test-secret",
		Audience: "platform-api", RequiredScope: "platform-access", AllowedClientIDs: []string{"browser-client"}}, bindings, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	r.now = func() time.Time { return time.Unix(1700000000, 0) }
	return r
}

func request() *http.Request {
	r := httptest.NewRequest("POST", "https://platform.example.test/review", strings.NewReader(`{"actor_id":"forged"}`))
	r.Header.Set("Authorization", "Bearer fixture-access-token")
	r.Header.Set("X-Actor-ID", "forged")
	r.Header.Set("X-Workspace-ID", uuid.NewString())
	return r
}

func jsonHandler(t *testing.T, claims any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/introspect" {
			t.Error("unexpected provider request")
		}
		id, secret, ok := r.BasicAuth()
		if !ok || id != "resource-client" || secret != "test-secret" {
			t.Error("SDK did not authenticate introspection client")
		}
		if r.ParseForm() != nil || r.Form.Get("token") != "fixture-access-token" {
			t.Error("SDK did not send the supplied access token")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(claims); err != nil {
			t.Error(err)
		}
	}
}

func TestSharedIntrospectionContract(t *testing.T) {
	f := contract(t)
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			claims := make(map[string]any)
			for key, value := range f.Claims {
				claims[key] = value
			}
			for key, value := range c.Set {
				claims[key] = value
			}
			for _, key := range c.Remove {
				delete(claims, key)
			}
			calls := 0
			r := newTestResolver(t, jsonHandler(t, claims), bindingFunc(func(_ context.Context, i Identity, w uuid.UUID) (Binding, error) {
				calls++
				if i != (Identity{"https://idp.example.test", "human-001"}) || w != workspace {
					t.Error("identity or workspace spoofed")
				}
				return activeBinding(), nil
			}))
			p, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision)
			if c.Accept || c.AcceptCore {
				if err != nil || p.ActorID != actor || p.Subject != "platform:person-001" || calls != 1 {
					t.Fatalf("expected mapped actor, got %+v / %v", p, err)
				}
			} else if err == nil || p.ActorID != uuid.Nil || calls != 0 {
				t.Fatalf("invalid identity reached authority owner: %+v / %v / %d", p, err, calls)
			}
		})
	}
}

func TestCurrentBindingAndRevocation(t *testing.T) {
	f := contract(t)
	b := activeBinding()
	var received Identity
	r := newTestResolver(t, jsonHandler(t, f.Claims), bindingFunc(func(_ context.Context, i Identity, _ uuid.UUID) (Binding, error) { received = i; return b, nil }))
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); err != nil {
		t.Fatal(err)
	}
	if received.Subject != "human-001" {
		t.Fatal("request or email minted identity")
	}
	b.Active = false
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); !errors.Is(err, principal.ErrUnauthenticated) {
		t.Fatal("stale active binding accepted")
	}
	b = activeBinding()
	b.WorkspaceID = uuid.New()
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); !errors.Is(err, principal.ErrWorkspaceDenied) {
		t.Fatal("cross-workspace binding accepted")
	}
	b = activeBinding()
	b.Capabilities = map[string]struct{}{}
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); !errors.Is(err, principal.ErrCapabilityDenied) {
		t.Fatal("provider admin role granted capability")
	}
	b = activeBinding()
	f.Claims["active"] = false
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); err == nil {
		t.Fatal("revoked token accepted from cache")
	}
}

func TestBindingAuthorityFailurePreservesErrorAndReturnsNoActor(t *testing.T) {
	f := contract(t)
	fault := errors.New("fixture binding authority unavailable")
	calls := 0
	r := newTestResolver(t, jsonHandler(t, f.Claims), bindingFunc(func(context.Context, Identity, uuid.UUID) (Binding, error) {
		calls++
		return activeBinding(), fault
	}))
	p, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision)
	if !errors.Is(err, fault) || calls != 1 || p.ActorID != uuid.Nil || p.Subject != "" || len(p.Capabilities) != 0 {
		t.Fatalf("binding failure lost or actor retained: %+v / %v / %d", p, err, calls)
	}
}

func TestProviderFailuresAndTransport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		typ, body string
	}{
		{"redirect", 302, "application/json", `{"active":true}`},
		{"unavailable", 503, "application/json", `{"error":"fixture-access-token test-secret"}`},
		{"wrong_media", 200, "text/html", "<html>error</html>"},
		{"oversized", 200, "application/json", strings.Repeat(" ", 65537)},
		{"trailing_json", 200, "application/json", `{} {}`},
		{"malformed", 200, "application/json", `{"active":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestResolver(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.typ)
				w.Header().Set("Location", "https://untrusted.example.test/")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}, bindingFunc(func(context.Context, Identity, uuid.UUID) (Binding, error) {
				t.Error("invalid provider response reached binding owner")
				return activeBinding(), nil
			}))
			_, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision)
			if !errors.Is(err, ErrProviderUnavailable) || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "fixture-access-token") {
				t.Fatalf("unsafe provider failure: %v", err)
			}
		})
	}
}

func TestHeaderAndConfigurationDenials(t *testing.T) {
	r := newTestResolver(t, func(http.ResponseWriter, *http.Request) { t.Error("malformed credential reached provider") }, bindingFunc(func(context.Context, Identity, uuid.UUID) (Binding, error) { return activeBinding(), nil }))
	for _, value := range []string{"", "Basic fixture-access-token", "Bearer", "Bearer a b", "Bearer a,b", "Bearer " + strings.Repeat("x", 8193)} {
		req := request()
		req.Header.Set("Authorization", value)
		if _, err := r.Resolve(req, workspace, principal.CapabilityHumanDecision); !errors.Is(err, principal.ErrUnauthenticated) {
			t.Fatalf("accepted header %q", value[:min(len(value), 20)])
		}
	}
	req := request()
	req.Header.Add("Authorization", "Bearer other")
	if _, err := r.Resolve(req, workspace, principal.CapabilityHumanDecision); err == nil {
		t.Fatal("duplicate authorization accepted")
	}
	c := r.config
	if strings.Contains(fmt.Sprintf("%+v %#v", c, c), "test-secret") {
		t.Fatal("configuration diagnostics expose credentials")
	}
	for _, endpoint := range []string{"http://idp.example.test", "https://user:secret@idp.example.test", "https://idp.example.test?a=b", "https://idp.example.test#fragment"} {
		c.Issuer = endpoint
		if _, err := NewResolver(c, r.bindings, nil); !errors.Is(err, principal.ErrNotConfigured) {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	c = r.config
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if _, err := NewResolver(c, r.bindings, client); !errors.Is(err, principal.ErrNotConfigured) {
		t.Fatal("insecure TLS accepted")
	}
	if _, err := NewResolver(c, nil, nil); !errors.Is(err, principal.ErrNotConfigured) {
		t.Fatal("missing binding owner accepted")
	}
}

func TestDistinctPeopleAndMissingBindings(t *testing.T) {
	f := contract(t)
	second := uuid.MustParse("20000000-0000-0000-0000-000000000002")
	failure := false
	r := newTestResolver(t, jsonHandler(t, f.Claims), bindingFunc(func(_ context.Context, i Identity, _ uuid.UUID) (Binding, error) {
		if failure {
			return Binding{}, errors.New("binding owner unavailable")
		}
		b := activeBinding()
		switch i.Subject {
		case "human-001":
			return b, nil
		case "human-002":
			b.ActorID = second
			b.Subject = "platform:person-002"
			return b, nil
		default:
			return Binding{}, nil
		}
	}))
	for subject, id := range map[string]uuid.UUID{"human-001": actor, "human-002": second} {
		f.Claims["sub"] = subject // Same email/roles; only the explicit subject binding changes.
		p, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision)
		if err != nil || p.ActorID != id {
			t.Fatalf("subjects were auto-linked: %+v / %v", p, err)
		}
	}
	f.Claims["sub"] = "unmapped"
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); err == nil {
		t.Fatal("missing mapping accepted")
	}
	failure = true
	if _, err := r.Resolve(request(), workspace, principal.CapabilityHumanDecision); err == nil {
		t.Fatal("failed binding owner accepted")
	}
}

func TestProviderDeadlineAndTLSValidation(t *testing.T) {
	r := newTestResolver(t, func(w http.ResponseWriter, req *http.Request) {
		select {
		case <-req.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}, bindingFunc(func(context.Context, Identity, uuid.UUID) (Binding, error) {
		t.Error("failed provider reached binding owner")
		return activeBinding(), nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Resolve(request().WithContext(ctx), workspace, principal.CapabilityHumanDecision); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatal("request deadline was not honored")
	}
	// An untrusted test certificate must fail; no global trust or TLS bypass.
	plain, err := NewResolver(r.config, r.bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Resolve(request(), workspace, principal.CapabilityHumanDecision); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatal("untrusted certificate accepted")
	}
}
