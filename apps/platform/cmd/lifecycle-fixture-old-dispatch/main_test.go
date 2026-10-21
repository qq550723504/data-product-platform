package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func observerConfig(at time.Time) config {
	return config{Enabled: true, AuditID: uuid.New(), WorkspaceID: uuid.MustParse("a4aa2689-6e8f-4180-a567-dfb875407eac"), ActorID: uuid.MustParse("033a56ad-99c2-43e3-89e0-a749979a8644"),
		Purpose: purpose, PostgresDSN: "postgres://fixture:test-only-password@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable", Core: pilot.PostV22Core,
		BinarySHA256: strings.Repeat("b", 64), PublicKey: strings.Repeat("c", 64), EvidenceSHA256: strings.Repeat("d", 64), Token: strings.Repeat("t", 32),
		Owners: map[string]string{strings.Repeat("a", 64): strings.Repeat("b", 64)}, NotBefore: at.Add(-time.Minute), ExpiresAt: at.Add(30 * time.Minute), MaxProviderDeletes: 0}
}

func TestOldDispatchObserverScopeAndPrincipalContainment(t *testing.T) {
	at := time.Now().UTC()
	c := observerConfig(at)
	if err := validate(c, at); err != nil {
		t.Fatal(err)
	}
	for _, capability := range []string{lc.CapabilityRevokeApproval, lc.CapabilityObserveAttempt} {
		p, err := (authority{c}).resolve(c, c.WorkspaceID, capability, at)
		if err != nil || p.WorkspaceID != c.WorkspaceID || p.ActorID != c.ActorID || len(p.Capabilities) != 2 || p.HasCapability(lc.CapabilityDispatch) {
			t.Fatal("bounded principal resolution failed", p, err)
		}
	}
	for _, capability := range []string{lc.CapabilityDispatch, lc.CapabilityApproveDisposition, lc.CapabilityReleaseHold, lc.CapabilityTerminateReader} {
		if _, err := (authority{c}).resolve(c, c.WorkspaceID, capability, at); err == nil {
			t.Fatal("expanded authority", capability)
		}
	}
	changes := map[string]func(*config){
		"expired":          func(c *config) { c.ExpiresAt = at },
		"future":           func(c *config) { c.NotBefore = at.Add(time.Second) },
		"window-extension": func(c *config) { c.ExpiresAt = at.Add(time.Hour) },
		"disabled":         func(c *config) { c.Enabled = false },
		"old-execution":    func(c *config) { c.AuditID = uuid.MustParse(pilot.PostV22Execution) },
		"workspace":        func(c *config) { c.WorkspaceID = uuid.New() },
		"Core":             func(c *config) { c.Core = strings.Repeat("e", 64) },
		"delete-budget":    func(c *config) { c.MaxProviderDeletes = 1 },
		"other-database":   func(c *config) { c.PostgresDSN = strings.Replace(c.PostgresDSN, "/lifecycle307", "/postgres", 1) },
		"other-user":       func(c *config) { c.PostgresDSN = strings.Replace(c.PostgresDSN, "fixture:", "admin:", 1) },
		"other-host":       func(c *config) { c.PostgresDSN = strings.Replace(c.PostgresDSN, "808a6960", "other", 1) },
		"fallback":         func(c *config) { c.PostgresDSN += "&host=other" },
		"dsn-options":      func(c *config) { c.PostgresDSN += "&options=-csearch_path=other" },
		"TLS":              func(c *config) { c.PostgresDSN = strings.Replace(c.PostgresDSN, "disable", "require", 1) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			bad := observerConfig(at)
			change(&bad)
			if err := validate(bad, at); err == nil {
				t.Fatal("invalid observer contract admitted")
			}
		})
	}
	changed := c
	changed.Token = strings.Repeat("s", 32)
	if _, err := (authority{c}).resolve(changed, c.WorkspaceID, lc.CapabilityObserveAttempt, at); err == nil {
		t.Fatal("current identity binding drift admitted")
	}
}

func TestOldDispatchV27CompositionIsFiniteAndObserveOnly(t *testing.T) {
	at := time.Now().UTC()
	c := observerConfig(at)
	c.Purpose = v27Purpose
	if err := validate(c, at); err != nil {
		t.Fatal(err)
	}
	verifier, targets, prefix := composition(c, nil, pilot.SignedClosedReferenceExecutor{})
	if _, ok := verifier.(pilot.PostV27NoSendVerifier); !ok || len(targets) != 3 || prefix != "post-v27-closed:" {
		t.Fatal("wrong finite composition")
	}
	if targets[0].Attempt.String() != "ba5bec6b-84ec-4418-a63f-4b08601ccb76" {
		t.Fatal("old execution selected")
	}
	for _, capability := range []string{lc.CapabilityDispatch, lc.CapabilityApproveDisposition, lc.CapabilityReleaseHold} {
		if _, err := (authority{c}).resolve(c, c.WorkspaceID, capability, at); err == nil {
			t.Fatal("expanded authority")
		}
	}
	for _, change := range []func(*config){func(c *config) { c.Purpose = "ARBITRARY_CLOSURE" }, func(c *config) { c.AuditID = uuid.MustParse(pilot.PostV27Execution) }, func(c *config) { c.MaxProviderDeletes = 1 }} {
		bad := c
		change(&bad)
		if validate(bad, at) == nil {
			t.Fatal("expanded contract")
		}
	}
}
