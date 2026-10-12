package storage

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"io"
	"net/http"
	"testing"
)

func TestProcessingOutputCreateOnlyExactPayloadSinglePhysicalAttempt(t *testing.T) {
	for _, scenario := range []string{"valid", "mismatch", "denied", "missing-version"} {
		t.Run(scenario, func(t *testing.T) {
			op := copyFixture()
			j := &inspectionTestJournal{}
			calls := 0
			b := copierTest(t, op, j, func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, e := io.ReadAll(r.Body)
				if e != nil || string(body) != "synthetic" || r.Method != "PUT" || r.URL.Path != "/"+op.Source.Namespace+"/fixture/output.csv" || r.URL.RawQuery != "" || r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Copy-Source") != "" {
					t.Error("output escaped create-only exact contract")
				}
				if scenario == "denied" {
					w.WriteHeader(503)
					fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
					return
				}
				if scenario != "missing-version" {
					w.Header().Set("X-Amz-Version-Id", "native-output-version")
				}
				w.WriteHeader(200)
			})
			writer := &LifecycleOutputWriter{b}
			intent := lc.ProcessingOutputIntent{ID: uuid.New(), WorkspaceID: op.WorkspaceID, Source: op.Source, DatasetVersionID: uuid.New(), DatasetID: uuid.New(), NamespaceID: uuid.New(), Namespace: op.Source.Namespace, Key: "fixture/output.csv", ContentSHA256: op.Source.ContentSHA256, ByteSize: op.Source.ByteSize}
			payload := []byte("synthetic")
			if scenario == "mismatch" {
				payload = []byte("wrongdata")
			}
			version, id, e := writer.PutPrepared(context.Background(), intent, payload)
			if scenario == "valid" {
				if e != nil || version != "native-output-version" || id == uuid.Nil {
					t.Fatal("exact native output identity missing", e)
				}
			} else if e == nil {
				t.Fatal("invalid output allowed")
			}
			if scenario == "mismatch" {
				if calls != 0 || len(j.starts) != 0 {
					t.Fatal("mismatched payload reached provider")
				}
			} else if calls != 1 || len(j.starts) != 1 || len(j.observations) != 1 || j.starts[0].OutputIntent == nil || j.starts[0].Operation != lc.StorageProcessingPut || j.observations[0].InvokedAt == nil {
				t.Fatal("output call was repeated or lacked durable attempt")
			}
		})
	}
}
