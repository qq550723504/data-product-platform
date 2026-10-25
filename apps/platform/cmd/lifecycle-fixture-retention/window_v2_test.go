package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionV2UsesOneBoundDateForPlanWireAndReadback(t *testing.T) {
	f := testFixture(t)
	f.b.RetainUntil = f.b.NotAfter.Add(48 * time.Hour)
	expected, err := proposalValue(f.b)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(expected)
	f.b.PlanSHA256 = digest(raw)
	for _, dir := range f.dirs {
		if os.WriteFile(filepath.Join(dir, "approved-proposal.json"), raw, 0600) != nil {
			t.Fatal("plan write")
		}
	}
	f.writeBindings(t)
	for _, group := range []string{"business-races", "mixed"} {
		if err := f.engine(t, group).run(context.Background()); err != nil {
			t.Fatal(group, err)
		}
		if f.expiries[group] != f.b.RetainUntil.UTC().Format(time.RFC3339) {
			t.Fatal("wire/readback used another cutoff")
		}
	}
	if f.puts != 2 {
		t.Fatal("not exactly two modeled PUTs", f.puts)
	}
}

func TestRetentionV2HorizonAndRehashedProposalDenialsBeforeIO(t *testing.T) {
	for _, fault := range []string{"short", "long", "fraction", "legacy-kind", "root-date", "object-date", "sdk-date", "extra-target", "extra-field"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			b := f.b
			if fault == "short" {
				b.RetainUntil = b.NotAfter.Add(2*time.Hour - time.Second)
			}
			if fault == "long" {
				b.RetainUntil = b.NotAfter.Add(72*time.Hour + time.Second)
			}
			if fault == "fraction" {
				b.RetainUntil = b.RetainUntil.Add(time.Nanosecond)
			}
			if fault == "legacy-kind" {
				b.Kind = "RETENTION_EXTENSION_EXACT_TWO_V1"
			}
			if fault == "short" || fault == "long" || fault == "fraction" || fault == "legacy-kind" {
				if checkedBindings(b) == nil {
					t.Fatal("unsafe binding")
				}
				return
			}
			p, _ := proposalValue(b)
			rows := p["objects"].([]any)
			switch fault {
			case "root-date":
				p["retain_until"] = "2026-10-25T12:40:00Z"
			case "object-date":
				rows[0].(map[string]any)["new_retain_until"] = "2026-10-25T12:40:00Z"
			case "sdk-date":
				rows[0].(map[string]any)["sdk_options"].(map[string]any)["RetainUntilDate"] = "2026-10-25T12:40:00Z"
			case "extra-target":
				p["objects"] = append(rows, rows[0])
			case "extra-field":
				p["Bypass"] = true
			}
			raw, _ := json.Marshal(p)
			b.PlanSHA256 = digest(raw)
			if checkedProposal(raw, b) == nil || f.requests != 0 {
				t.Fatal("rehashed expanded proposal admitted")
			}
		})
	}
}

func TestRetentionWaitReadyIsOnceAfterFalseAndPersistenceFailureNeverEnables(t *testing.T) {
	for _, fault := range []string{"success", "persist-failure", "missing-callback"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			dir := f.dirs["business-races"]
			raw, _ := os.ReadFile(filepath.Join(dir, "bindings.json"))
			hash := digest(raw)
			writeJSON(t, filepath.Join(dir, "permission.json"), permission{false, hash})
			calls := 0
			var ready func() error
			if fault != "missing-callback" {
				ready = func() error {
					calls++
					var p permission
					if readPermission(filepath.Join(dir, "permission.json"), &p) != nil || p.Enabled {
						t.Fatal("ready before observing false")
					}
					if fault == "persist-failure" {
						return errors.New("directory fsync unavailable")
					}
					writeJSON(t, filepath.Join(dir, "permission.json"), permission{true, hash})
					return nil
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := waitPermission(ctx, dir, raw, func() time.Time { return f.at }, time.Millisecond, ready)
			if (err == nil) != (fault == "success") || f.requests != 0 {
				t.Fatal("wait bypass or provider IO", err)
			}
			expected := 1
			if fault == "missing-callback" {
				expected = 0
			}
			if calls != expected {
				t.Fatal("ready callback replay", calls)
			}
		})
	}
}
