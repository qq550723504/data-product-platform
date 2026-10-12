package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptrace"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

type custodyWriteFailureTransport struct {
	scenario string
}

func (f custodyWriteFailureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	trace := httptrace.ContextClientTrace(r.Context())
	switch f.scenario {
	case "write-failed":
		trace.WroteRequest(httptrace.WroteRequestInfo{Err: errors.New("synthetic request write failure")})
	case "response-lost":
		trace.WroteRequest(httptrace.WroteRequestInfo{})
	case "concurrent-callbacks":
		var done sync.WaitGroup
		for i := 0; i < 16; i++ {
			done.Add(1)
			go func() {
				defer done.Done()
				trace.WroteRequest(httptrace.WroteRequestInfo{})
			}()
		}
		done.Wait()
	}
	return nil, errors.New("synthetic transport failure")
}

func TestCustodyTransportInvocationRequiresSuccessfulRequestWrite(t *testing.T) {
	for _, operation := range []string{"copy", "verify", "read", "output"} {
		for _, scenario := range []string{"before-write", "write-failed", "response-lost", "concurrent-callbacks"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				op := copyFixture()
				journal := &inspectionTestJournal{}
				backend := copierTest(t, op, journal, func(http.ResponseWriter, *http.Request) {
					t.Error("synthetic transport must not reach an HTTP server")
				})
				backend.transport.base = custodyWriteFailureTransport{scenario: scenario}
				var err error
				switch operation {
				case "copy":
					_, _, err = backend.CreateCopy(context.Background(), op)
				case "verify":
					_, err = backend.VerifyCopyBytes(context.Background(), op, op.Materialized("synthetic-target"))
				case "read":
					reader := &LifecycleReader{backend: backend, copies: map[uuid.UUID]domain.Copy{op.Source.ID: op.Source}}
					pin := domain.ReaderPin{ID: uuid.New(), WorkspaceID: op.WorkspaceID, CopyID: op.Source.ID, ReaderID: uuid.New(), Kind: domain.FixtureReader, ActorID: op.ActorID, PrincipalRef: op.PrincipalRef, Generation: 2, GateRevision: 1, StartedAt: time.Now().Add(-time.Second), WindowMode: domain.UseUnbounded}
					_, err = reader.ReadExactPinned(context.Background(), op.Source, pin)
				case "output":
					writer := &LifecycleOutputWriter{backend: backend}
					intent := lc.ProcessingOutputIntent{ID: uuid.New(), WorkspaceID: op.WorkspaceID, Source: op.Source, DatasetVersionID: uuid.New(), DatasetID: uuid.New(), NamespaceID: uuid.New(), Namespace: op.Source.Namespace, Key: "fixture/output.csv", ContentSHA256: op.Source.ContentSHA256, ByteSize: op.Source.ByteSize}
					_, _, err = writer.PutPrepared(context.Background(), intent, []byte("synthetic"))
				}
				if err == nil || len(journal.starts) != 1 || len(journal.observations) != 1 {
					t.Fatalf("failure must retain one start and observation: err=%v starts=%d observations=%d", err, len(journal.starts), len(journal.observations))
				}
				observation := journal.observations[0]
				wantInvoked := scenario == "response-lost" || scenario == "concurrent-callbacks"
				if observation.Failure != "TRANSPORT" || (observation.InvokedAt != nil) != wantInvoked {
					t.Fatalf("invocation evidence must reflect successful writes only: %+v", observation)
				}
			})
		}
	}
}
