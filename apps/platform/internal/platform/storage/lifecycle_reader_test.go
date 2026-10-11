package storage

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestPinnedReaderExactGetSingleSendAndPositiveStop(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong-version", "wrong-bytes", "denied", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			op := copyFixture()
			c := op.Source
			j := &inspectionTestJournal{}
			requests := 0
			b := copierTest(t, op, j, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || r.URL.Query().Get("versionId") != c.ProviderVersion || r.URL.Query().Get("versions") != "" || r.Header.Get("X-Amz-Copy-Source") != "" {
					t.Error("unpinned/mutating request")
				}
				version := c.ProviderVersion
				if scenario == "wrong-version" {
					version = "other-version"
				}
				w.Header().Set("X-Amz-Version-Id", version)
				if scenario == "denied" {
					w.WriteHeader(403)
					return
				}
				payload := "synthetic"
				if scenario == "wrong-bytes" {
					payload = "wrongdata"
				}
				fmt.Fprint(w, payload)
			})
			s := &LifecycleReader{backend: b, copies: map[uuid.UUID]domain.Copy{c.ID: c}}
			pin := domain.ReaderPin{ID: uuid.New(), WorkspaceID: c.WorkspaceID, CopyID: c.ID, ReaderID: uuid.New(), Kind: domain.FixtureReader, ActorID: uuid.New(), PrincipalRef: "controlled", Generation: 2, GateRevision: 1, StartedAt: time.Now().Add(-time.Second), WindowMode: domain.UseUnbounded}
			if scenario == "foreign" {
				c.ProviderVersion = "foreign-version"
			}
			p, e := s.ReadExactPinned(context.Background(), c, pin)
			if scenario == "valid" {
				if e != nil || string(p.Bytes) != "synthetic" || p.StoppedAt.IsZero() || len(j.starts) != 1 || j.starts[0].ReaderPin == nil || j.starts[0].CopyIntent != nil || j.starts[0].Operation != lc.StorageReadPinned {
					t.Fatal("missing pinned result/start/stop", p, e)
				}
			} else if e == nil {
				t.Fatal("invalid response/scope allowed")
			}
			if scenario == "foreign" {
				if requests != 0 {
					t.Fatal("foreign scope reached provider")
				}
			} else if requests != 1 || len(j.observations) != 1 || j.observations[0].InvokedAt == nil {
				t.Fatal("physical read/cost proof repeated or missing")
			}
		})
	}
}
