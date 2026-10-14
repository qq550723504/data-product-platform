package main

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"strings"
	"testing"
	"time"
)

func TestReferenceSendPermissionIsDistinctBoundedAndDisabledByDefault(t *testing.T) {
	at := time.Now().UTC()
	digest := strings.Repeat("a", 64)
	c := referenceConfig{}
	c.Fixture.InspectAccess = "inspectfixture"
	p := referencePermission{permission: permission{Enabled: true, RunID: uuid.MustParse(pilot.ReferenceExecution), WorkspaceID: pilot.ReferenceWorkspace, PlanHash: pilot.ReferencePlanHash, DeleteAccess: "refdelete614cd9ad", DeleteSecret: "secret", BasisRef: "human-confirmed:reference-disposal:" + digest, TopologySHA256: digest, ConfirmedAt: at.Add(-time.Minute), ExpiresAt: at.Add(time.Minute)}, ConfirmationSHA256: digest, BinarySHA256: digest, ProfileSHA256: referenceTargetsDigest(), MaxTotal: 2, MaxPerTarget: 1}
	if e := validateReferencePermission(p, c, at); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*referencePermission){func(p *referencePermission) { p.Enabled = false }, func(p *referencePermission) { p.RunID = uuid.MustParse(pilot.ReferenceEnvironment) }, func(p *referencePermission) { p.PlanHash = strings.Repeat("b", 64) }, func(p *referencePermission) { p.ProfileSHA256 = strings.Repeat("b", 64) }, func(p *referencePermission) { p.ExpiresAt = at.Add(time.Hour) }, func(p *referencePermission) { p.ConfirmedAt = at.Add(time.Second) }, func(p *referencePermission) { p.MaxTotal = 3 }, func(p *referencePermission) { p.MaxPerTarget = 2 }, func(p *referencePermission) { p.ConfirmationSHA256 = "" }, func(p *referencePermission) { p.BasisRef = "CONTROLLED_NON_DELETE_ADMISSION_EXERCISE" }}
	for _, mutate := range mutations {
		changed := p
		mutate(&changed)
		if validateReferencePermission(changed, c, at) == nil {
			t.Fatal("expanded/old/disabled permission allowed")
		}
	}
	c.Fixture.RootAccess = "root"
	if validateReferencePermission(p, c, at) == nil {
		t.Fatal("root payload composition accepted")
	}
	copies := pilot.ReferenceCopies()
	targets := pilot.ReferenceTargets()
	if len(copies) != 6 || len(targets) != 2 {
		t.Fatal("scope expanded")
	}
	for _, target := range targets {
		if target.Key != "fixture/a.csv" && target.Key != "fixture/b.csv" {
			t.Fatal("output selected")
		}
	}
}
