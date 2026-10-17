package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestPreviousExecutionCannotBeReauthorizedByNewBinary(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../docs/acceptance/evidence/issue307-d1/remaining-candidate07/candidate.json"))
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
	l := loaded{Scope: scope, F: config{PostgresDSN: "postgres://fixture:synthetic@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable", InspectAccess: "inspect808a69606c36", InspectSecret: "synthetic", DeleteAccess: "delete808a6960", Token: "synthetic"}}
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
