package storage

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"net/http"
	"testing"
)

func TestCheckpointExactReadHasMaintenanceSubjectAndNeverRetries(t *testing.T) {
	for _, scenario := range []string{"valid", "wrong-version", "wrong-hash", "denied", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			op := copyFixture()
			c := op.Source
			j := &inspectionTestJournal{}
			calls := 0
			backend := copierTest(t, op, j, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Query().Get("versionId") != c.ProviderVersion || len(r.URL.Query()) != 1 {
					t.Error("expanded checkpoint request")
				}
				v := c.ProviderVersion
				if scenario == "wrong-version" {
					v = "foreign-version"
				}
				w.Header().Set("X-Amz-Version-Id", v)
				if scenario == "denied" {
					w.WriteHeader(403)
					return
				}
				payload := "synthetic"
				if scenario == "wrong-hash" {
					payload = "wrongdata"
				}
				fmt.Fprint(w, payload)
			})
			s := &LifecycleCheckpointReader{&LifecycleReader{backend: backend, copies: map[uuid.UUID]domain.Copy{c.ID: c}}}
			if scenario == "foreign" {
				c.ProviderVersion = "foreign-version"
			}
			data, _, e := s.ReadExact(context.Background(), c)
			if scenario == "valid" {
				if e != nil || string(data) != "synthetic" || len(j.starts) != 1 || j.starts[0].Operation != lc.StorageReadCheckpoint || j.starts[0].ReaderPin != nil || j.starts[0].CopyIntent != nil || j.starts[0].CopyOperationID != uuid.Nil {
					t.Fatal("missing maintenance subject", e)
				}
			} else if e == nil {
				t.Fatal("invalid backup accepted")
			}
			if scenario == "foreign" {
				if calls != 0 {
					t.Fatal("foreign read sent")
				}
			} else if calls != 1 || len(j.observations) != 1 || j.observations[0].InvokedAt == nil {
				t.Fatal("lost or repeated physical invocation")
			}
		})
	}
}
