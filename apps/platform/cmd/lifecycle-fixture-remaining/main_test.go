package main

import (
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestFrozenAbsoluteWindowCannotBeMovedExtendedOrReopened(t *testing.T) {
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
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
