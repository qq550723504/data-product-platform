package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func fixtureCopy(workspace uuid.UUID) Copy {
	return Copy{ID: uuid.New(), WorkspaceID: workspace, DatasetVersionID: uuid.New(), ProviderInstanceID: uuid.New(),
		IncarnationID: uuid.New(), Namespace: "synthetic-online", Key: "fixture/合成:only.csv", ProviderVersion: "fixed-version-1",
		ContentSHA256: strings.Repeat("a", 64), ByteSize: 12}
}

func frozenPlan(t *testing.T) Plan {
	t.Helper()
	w := uuid.New()
	before := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	p := Plan{ID: uuid.New(), WorkspaceID: w, Action: DeleteExact, Status: Finalized, PreparedBy: uuid.New(),
		PrincipalRef: "controlled-operator", CreatedAt: before, FinalizedAt: &before, Targets: []Copy{fixtureCopy(w)}}
	var err error
	p.ContentHash, err = ScopeHash(w, p.Action, p.Targets)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScopeHashCanonicalOrderAndExactVersion(t *testing.T) {
	w := uuid.New()
	a, b := fixtureCopy(w), fixtureCopy(w)
	first := []Copy{a, b}
	h1, err := ScopeHash(w, DeleteExact, first)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ScopeHash(w, DeleteExact, []Copy{b, a})
	if err != nil || h1 != h2 || first[0] != a {
		t.Fatal("canonical hash changed order or input")
	}
	b.ProviderVersion = "fixed-version-2"
	h3, err := ScopeHash(w, DeleteExact, []Copy{a, b})
	if err != nil || h3 == h1 {
		t.Fatal("hash did not bind exact provider version")
	}
	b.IncarnationID = uuid.New()
	h4, _ := ScopeHash(w, DeleteExact, []Copy{a, b})
	if h4 == h3 {
		t.Fatal("hash did not bind provider incarnation")
	}
}

func TestScopeHashRejectsBareKeyDuplicateForeignAndInvalid(t *testing.T) {
	w := uuid.New()
	base := fixtureCopy(w)
	for _, test := range []struct {
		name   string
		mutate func(*Copy)
	}{
		{"bare key", func(c *Copy) { c.ProviderVersion = "" }},
		{"null version", func(c *Copy) { c.ProviderVersion = "null" }},
		{"unknown instance", func(c *Copy) { c.IncarnationID = uuid.Nil }},
		{"foreign", func(c *Copy) { c.WorkspaceID = uuid.New() }},
		{"etag", func(c *Copy) { c.ContentSHA256 = "etag" }},
		{"zero bytes", func(c *Copy) { c.ByteSize = 0 }},
		{"unbounded", func(c *Copy) { c.ByteSize = MaxFixtureBytes + 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := base
			test.mutate(&c)
			if _, err := ScopeHash(w, DeleteExact, []Copy{c}); err == nil {
				t.Fatal("invalid copy accepted")
			}
		})
	}
	if _, err := ScopeHash(w, DeleteExact, []Copy{base, base}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := ScopeHash(w, DeleteExact, nil); err == nil {
		t.Fatal("empty accepted")
	}
}

func eligibleInput(t *testing.T) DeletePrerequisites {
	p := frozenPlan(t)
	at := p.CreatedAt.Add(time.Hour)
	return DeletePrerequisites{AsOf: at, Plan: p,
		Approval:      Approval{WorkspaceID: p.WorkspaceID, PlanHash: p.ContentHash, Action: DeleteExact, ValidFrom: p.CreatedAt, ExpiresAt: at.Add(time.Hour), Current: Satisfied},
		Retention:     []RetentionBoundary{{PolicyID: uuid.New(), StartEventID: uuid.New(), StartsAt: p.CreatedAt, RetainUntil: at}},
		DisposalDueAt: at, CallerAuthority: Satisfied, UseRestricted: Satisfied, InventoryComplete: Satisfied,
		ReferencesReleased: Satisfied, ReadersDrained: Satisfied, HoldFree: Satisfied, ProviderRetentionAllows: Satisfied, LedgerCurrent: Satisfied}
}

func TestDeletePrerequisitesFailClosedForEveryCurrentDependency(t *testing.T) {
	base := eligibleInput(t)
	if d := EvaluateDeletePrerequisites(base); !d.Eligible || len(d.Blockers) != 0 {
		t.Fatalf("valid scope rejected: %+v", d)
	}
	for _, test := range []struct {
		name  string
		field func(*DeletePrerequisites) *Check
	}{
		{"authority", func(i *DeletePrerequisites) *Check { return &i.CallerAuthority }},
		{"restriction", func(i *DeletePrerequisites) *Check { return &i.UseRestricted }},
		{"inventory", func(i *DeletePrerequisites) *Check { return &i.InventoryComplete }},
		{"shared/material reference", func(i *DeletePrerequisites) *Check { return &i.ReferencesReleased }},
		{"reader or timed-out lease", func(i *DeletePrerequisites) *Check { return &i.ReadersDrained }},
		{"hold", func(i *DeletePrerequisites) *Check { return &i.HoldFree }},
		{"provider retention", func(i *DeletePrerequisites) *Check { return &i.ProviderRetentionAllows }},
		{"restore ledger", func(i *DeletePrerequisites) *Check { return &i.LedgerCurrent }},
	} {
		for _, state := range []Check{"", Unknown, Denied, "ANY"} {
			t.Run(test.name+"/"+string(state), func(t *testing.T) {
				in := base
				*test.field(&in) = state
				if d := EvaluateDeletePrerequisites(in); d.Eligible || len(d.Blockers) == 0 {
					t.Fatal("unknown/denied dependency allowed")
				}
			})
		}
	}
}

func TestRetentionAndApprovalCannotBeInferredFromOtherFacts(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*DeletePrerequisites)
	}{
		{"future retain", func(i *DeletePrerequisites) {
			r := i.Retention[0]
			r.RetainUntil = i.AsOf.Add(time.Second)
			i.Retention = []RetentionBoundary{r}
		}},
		{"second policy retained", func(i *DeletePrerequisites) {
			r := i.Retention[0]
			r.PolicyID = uuid.New()
			r.RetainUntil = i.AsOf.Add(time.Hour)
			i.Retention = append(append([]RetentionBoundary(nil), i.Retention...), r)
		}},
		{"unknown start", func(i *DeletePrerequisites) {
			r := i.Retention[0]
			r.StartEventID = uuid.Nil
			i.Retention = []RetentionBoundary{r}
		}},
		{"unknown policy", func(i *DeletePrerequisites) { i.Retention = nil }},
		{"expired approval", func(i *DeletePrerequisites) { i.Approval.ExpiresAt = i.AsOf }},
		{"future approval", func(i *DeletePrerequisites) { i.Approval.ValidFrom = i.AsOf.Add(time.Second) }},
		{"foreign approval", func(i *DeletePrerequisites) { i.Approval.WorkspaceID = uuid.New() }},
		{"wrong approval scope", func(i *DeletePrerequisites) { i.Approval.PlanHash = strings.Repeat("b", 64) }},
		{"wrong approval action", func(i *DeletePrerequisites) { i.Approval.Action = "USE" }},
		{"unknown due", func(i *DeletePrerequisites) { i.DisposalDueAt = time.Time{} }},
		{"draft", func(i *DeletePrerequisites) { i.Plan.Status = Draft }},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := eligibleInput(t)
			test.mutate(&in)
			if EvaluateDeletePrerequisites(in).Eligible {
				t.Fatal("invalid prerequisites allowed")
			}
		})
	}
}
