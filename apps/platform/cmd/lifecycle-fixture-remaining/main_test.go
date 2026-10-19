package main

import (
	"encoding/json"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestCurrentSixCandidatePositiveAndNoP1OrRestoreSourceSubstitution(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../docs/acceptance/evidence/issue307-d1/remaining-candidate12/candidate.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c candidate
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("historical metadata")
	}
	c.Execution, c.Profile = execution, profile
	c.NotBefore, _ = time.Parse(time.RFC3339, pilot.RemainingNotBefore)
	c.NotAfter, _ = time.Parse(time.RFC3339, pilot.RemainingNotAfter)
	var targets []domain.Copy
	for i := range c.Groups {
		g := &c.Groups[i]
		if g.Group == "business-races" {
			delete(g.Helpers, "P1")
			delete(g.Helpers, "recover-P1")
			delete(g.ConfigSHA256, "P1")
			delete(g.ConfigSHA256, "recover-P1")
			delete(g.CaseHelpers, "renew")
			delete(g.CaseConfigSHA256, "renew")
			p1, _ := historicalCopy(g.Group, "P1")
			delete(g.PolicyHeads, p1.DatasetVersionID.String())
		}
		for _, stage := range []string{"P2", "T1", "T2", "H2", "M1A", "M1B"} {
			if cp, e := fixedTarget(g.Group, stage); e == nil {
				targets = append(targets, cp)
			}
		}
	}
	raw, _ = json.Marshal(targets)
	c.TargetDigest = digest(raw)
	if len(targets) != 6 || validateCandidate(c) != nil {
		t.Fatal("valid six metadata rejected")
	}
	if _, err = fixedTarget("business-races", "P1"); err == nil {
		t.Fatal("P1 revived as a current target")
	}
	c.Groups[0].Native = c.Groups[0].CheckpointNative
	if validateCandidate(c) == nil {
		t.Fatal("restore substituted for original source")
	}
}

func TestPreviousExecutionCannotBeReauthorizedByNewBinary(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	for _, generation := range []string{"remaining-candidate07", "remaining-candidate09", "remaining-candidate10", "remaining-candidate11", "remaining-candidate12"} {
		t.Run(generation, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../docs/acceptance/evidence/issue307-d1", generation, "candidate.json"))
			if err != nil {
				t.Fatal(err)
			}
			var previous candidate
			if json.Unmarshal(raw, &previous) != nil {
				t.Fatal("previous candidate invalid")
			}
			if validateCandidate(previous) == nil {
				t.Fatal("previous execution/window accepted by current binary")
			}
			if previous.Execution == execution || previous.Profile == profile {
				t.Fatal("new generation reused previous authority identity")
			}
		})
	}
}

func TestFrozenAbsoluteWindowCannotBeMovedExtendedOrReopened(t *testing.T) {
	start, err := time.Parse(time.RFC3339, pilot.RemainingNotBefore)
	if err != nil {
		t.Fatal(err)
	}
	c := candidate{NotBefore: start, NotAfter: start.Add(40 * time.Minute)}
	p := permission{NotBefore: start, ExpiresAt: c.NotAfter, ConfirmedAt: start.Add(-time.Minute)}
	if validateWindow(c, p, start) != nil {
		t.Fatal("valid frozen window rejected")
	}
	for _, delta := range []time.Duration{time.Minute, time.Hour, 24 * time.Hour} {
		q := p
		q.NotBefore = q.NotBefore.Add(delta)
		q.ExpiresAt = q.ExpiresAt.Add(delta)
		if validateWindow(c, q, start.Add(delta)) == nil {
			t.Fatal("same confirmation shifted window")
		}
		q = p
		q.ExpiresAt = q.ExpiresAt.Add(delta)
		if validateWindow(c, q, start) == nil {
			t.Fatal("extended window")
		}
	}
	if validateWindow(c, p, c.NotAfter) == nil || validateWindow(c, p, start.Add(-time.Nanosecond)) == nil {
		t.Fatal("window boundary not strict")
	}
}
func TestReadAndSendShareStrictSourceCoreAndInspectionBinding(t *testing.T) {
	scope, _ := pilot.RemainingScopeFor("business-races")
	l := loaded{Scope: scope, F: config{PostgresDSN: "postgres://fixture:synthetic@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable", InspectAccess: pilot.RemainingIdentity(scope.Group, "inspect"), InspectSecret: "synthetic", DeleteAccess: pilot.RemainingIdentity(scope.Group, "delete"), Token: "synthetic"}}
	if strictConfig(l) != nil {
		t.Fatal("correct physical scope rejected")
	}
	for _, changed := range []string{"postgres://fixture:synthetic@127.0.0.1:5432/lifecycle307?sslmode=disable", strings.Replace(l.F.PostgresDSN, "-postgres:", "-remaining-regression-pg:", 1), strings.Replace(l.F.PostgresDSN, "/lifecycle307?", "/postgres?", 1)} {
		q := l
		q.F.PostgresDSN = changed
		if strictConfig(q) == nil {
			t.Fatal("wrong Core accepted")
		}
	}
	l.F.InspectAccess = "foreign-inspector"
	if strictConfig(l) == nil {
		t.Fatal("foreign IAM identity accepted")
	}
}
