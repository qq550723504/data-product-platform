package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestP1ReadOnlyWorkerRejectsAuthorityExpansionBeforeDatabase(t *testing.T) {
	at := time.Now().UTC()
	id := uuid.New()
	s, _ := pilot.RemainingScopeFor("business-races")
	c := p1AuditConfig{AuditID: id, Purpose: "POST_V22_P1_READONLY_RECONCILIATION", PostgresDSN: "postgres://fixture:test-only@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable", RedisAddr: "dpp307-p1-readonly-" + id.String()[:8] + "-redis:6379", RecoveryConfig: "/run/private/recovery.json", BinarySHA256: strings.Repeat("a", 64), Core: "9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0", Native: "f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce", NotBefore: at.Add(-time.Second), ExpiresAt: at.Add(30 * time.Minute)}
	r := pilot.RecoveryConfig{Enabled: true, SyntheticOnly: true, RunID: id, WorkspaceID: s.Workspace, ProviderInstanceID: s.Provider, IncarnationID: s.Incarnation, ActorID: uuid.MustParse("033a56ad-99c2-43e3-89e0-a749979a8644"), Endpoint: "127.0.0.1:9000", InspectAccess: "inspect808a69606c36", InspectSecret: "test-only", Token: "test-only", ManifestSHA256: s.ManifestSHA256, ValidUntil: c.ExpiresAt}
	if err := validateP1Audit(c, r, at); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*p1AuditConfig, *pilot.RecoveryConfig){
		"old-run":             func(c *p1AuditConfig, _ *pilot.RecoveryConfig) { c.AuditID = uuid.MustParse(pilot.RemainingExecution) },
		"foreign-workspace":   func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.WorkspaceID = uuid.New() },
		"foreign-provider":    func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.ProviderInstanceID = uuid.New() },
		"foreign-incarnation": func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.IncarnationID = uuid.New() },
		"root-identity":       func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.InspectAccess = "root" },
		"delete-capability":   func(c *p1AuditConfig, _ *pilot.RecoveryConfig) { c.MaxProviderDeletes = 1 },
		"disabled":            func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.Enabled = false },
		"different-deadline":  func(_ *p1AuditConfig, r *pilot.RecoveryConfig) { r.ValidUntil = r.ValidUntil.Add(time.Second) },
		"expanded-deadline": func(c *p1AuditConfig, r *pilot.RecoveryConfig) {
			c.ExpiresAt = c.NotBefore.Add(time.Hour)
			r.ValidUntil = c.ExpiresAt
		},
		"expired":       func(c *p1AuditConfig, r *pilot.RecoveryConfig) { c.ExpiresAt = at; r.ValidUntil = at },
		"another-queue": func(c *p1AuditConfig, _ *pilot.RecoveryConfig) { c.RedisAddr = "old-retained-redis:6379" },
		"another-Core-host": func(c *p1AuditConfig, _ *pilot.RecoveryConfig) {
			c.PostgresDSN = strings.Replace(c.PostgresDSN, "dpp-lifecycle307-808a6960-postgres", "checkpoint", 1)
		},
		"another-Core-user": func(c *p1AuditConfig, _ *pilot.RecoveryConfig) {
			c.PostgresDSN = strings.Replace(c.PostgresDSN, "fixture:", "root:", 1)
		},
		"another-Core-database": func(c *p1AuditConfig, _ *pilot.RecoveryConfig) {
			c.PostgresDSN = strings.Replace(c.PostgresDSN, "/lifecycle307?", "/postgres?", 1)
		},
		"Core-query-override": func(c *p1AuditConfig, _ *pilot.RecoveryConfig) { c.PostgresDSN += "&host=foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			cc, rr := c, r
			mutate(&cc, &rr)
			if validateP1Audit(cc, rr, at) == nil {
				t.Fatal("expanded worker authority admitted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "audit.json")
	raw, _ := json.Marshal(c)
	raw = append(raw[:len(raw)-1], []byte(",\"RootSecret\":\"forbidden\"}")...)
	os.WriteFile(path, raw, 0600)
	if err := runP1PostV22(context.Background(), path); !errors.Is(err, lc.ErrStorageScope) {
		t.Fatal("root field reached database or provider", err)
	}
}

type p1FakeInspector struct {
	protections                                   []lc.CopyProtection
	presentP1, omitSurvivor, duplicate, holdDrift bool
	calls                                         int
}

func (f *p1FakeInspector) InspectExactCopy(_ context.Context, c domain.Copy) (lc.ExactCopyInspection, error) {
	f.calls++
	return lc.ExactCopyInspection{Copy: c}, nil
}
func (f *p1FakeInspector) InspectRetentionAndHold(_ context.Context, c domain.Copy) (lc.CopyProtection, error) {
	for _, v := range f.protections {
		if v.Copy == c {
			if f.holdDrift {
				v.LegalHold = !v.LegalHold
			}
			return v, nil
		}
	}
	return lc.CopyProtection{}, lc.ErrStorageScope
}
func (f *p1FakeInspector) InspectNamespace(_ context.Context, c domain.Copy) (lc.NamespaceInventory, error) {
	r := lc.NamespaceInventory{}
	for _, v := range f.protections {
		cp := v.Copy
		if cp.Namespace != c.Namespace || (!f.presentP1 && cp.ID.String() == p1Copy) {
			continue
		}
		r.Versions = append(r.Versions, lc.ListedVersion{Key: cp.Key, Version: cp.ProviderVersion, ByteSize: cp.ByteSize})
	}
	if f.omitSurvivor && len(r.Versions) > 0 {
		r.Versions = r.Versions[1:]
	}
	if f.duplicate && len(r.Versions) > 0 {
		r.Versions = append(r.Versions, r.Versions[0])
	}
	return r, nil
}
func TestP1ReadOnlyNativePrecheckRequiresExactAbsenceAndEveryRetainedVersion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "acceptance", "evidence", "issue307-d1", "remaining-candidate12", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b []struct {
		Group  string `json:"group"`
		Copies []struct {
			Copy        domain.Copy
			LegalHold   bool
			Retention   string
			RetainUntil *time.Time
		} `json:"copy_protection"`
	}
	if json.Unmarshal(raw, &b) != nil {
		t.Fatal("fixture")
	}
	var p []lc.CopyProtection
	for _, v := range b[0].Copies {
		p = append(p, lc.CopyProtection{Copy: v.Copy, LegalHold: v.LegalHold, RetentionMode: v.Retention, RetainUntil: v.RetainUntil})
	}
	f := &p1FakeInspector{protections: p}
	if _, err = p1NativePrecheck(context.Background(), f, p); err != nil || f.calls != 7 {
		t.Fatal("bounded seven retained versions failed", err)
	}
	for _, name := range []string{"P1-present", "missing-survivor", "duplicate", "hold-drift"} {
		t.Run(name, func(t *testing.T) {
			x := &p1FakeInspector{protections: p, presentP1: name == "P1-present", omitSurvivor: name == "missing-survivor", duplicate: name == "duplicate", holdDrift: name == "hold-drift"}
			if _, err := p1NativePrecheck(context.Background(), x, p); err == nil {
				t.Fatal("invalid namespace/protection passed")
			}
		})
	}
	if err = (p1SelectedRecovery{workspace: uuid.New(), work: uuid.New()}).RunRecovery(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, lc.ErrStorageScope) {
		t.Fatal("foreign queue work admitted", err)
	}
}
