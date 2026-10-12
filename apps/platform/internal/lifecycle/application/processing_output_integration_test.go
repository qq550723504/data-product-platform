package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

type sqlOutputStore struct {
	journal  FixtureOutputJournal
	source   domain.Copy
	slot     ProcessingOutputSlot
	afterPut func()
	puts     int
}

func (s *sqlOutputStore) Put(context.Context, string, io.Reader, int64, string) (string, error) {
	return "", fmt.Errorf("%w: untyped test output method", ErrStorageScope)
}
func (s *sqlOutputStore) PutVersion(ctx context.Context, v datasetdomain.DatasetVersion, _ string, reader io.Reader, size int64, _ string) (string, error) {
	b, e := io.ReadAll(reader)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	i := ProcessingOutputIntent{ID: uuid.New(), WorkspaceID: s.source.WorkspaceID, DatasetVersionID: v.ID, DatasetID: v.DatasetID, NamespaceID: s.slot.NamespaceID, Namespace: s.slot.Namespace, Key: s.slot.Key, ContentSHA256: hex.EncodeToString(sum[:]), ByteSize: size, Source: s.source}
	call := uuid.New()
	if e = s.journal.StartStorageCall(ctx, StorageCallStart{ID: call, Copy: s.source, Operation: StorageProcessingPut, OutputIntent: &i, StartedAt: time.Now().UTC()}); e != nil {
		return "", e
	}
	s.puts++
	at := time.Now().UTC()
	if e = s.journal.ObserveStorageCall(ctx, StorageCallObservation{CallID: call, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: hex.EncodeToString(sum[:]), CopyResultVersion: "SQL-only-output-version"}); e != nil {
		return "", e
	}
	if s.afterPut != nil {
		s.afterPut()
	}
	return "s3://" + s.slot.Namespace + "/" + s.slot.Key, nil
}
func TestProcessingOutputReservedBeforePutAndReadyUsesSameFence(t *testing.T) {
	for _, mode := range []string{"current", "restriction-before-publish"} {
		t.Run(mode, func(t *testing.T) {
			pool, w, c, o, reader, _, _ := newReaderFixture(t)
			until := time.Now().UTC().Add(30 * time.Minute)
			o.useGate.(*occupancyUseGate).hook = func(_ context.Context, _ pgx.Tx, _ UseGateRequest, d *domain.UseDecision) error {
				d.WindowMode = domain.UseDeadline
				d.UseUntil = &until
				return nil
			}
			ex, _ := subjectExecution(t, pool, w, c, o)
			pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
			var ns uuid.UUID
			if e := pool.QueryRow(context.Background(), `SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$1`, c.ID).Scan(&ns); e != nil {
				t.Fatal(e)
			}
			slot := ProcessingOutputSlot{DatasetID: ex.OutputDatasetID, NamespaceID: ns, Namespace: c.Namespace, Key: "sql-only/output.csv"}
			journal := FixtureOutputJournal{pins, []ProcessingOutputSlot{slot}, NewPostgresStorageCallJournal(o)}
			store := &sqlOutputStore{journal: journal, source: c, slot: slot}
			if mode == "restriction-before-publish" {
				store.afterPut = func() {
					if _, e := o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: CommandHeader{w, "late-output-restriction"}, DatasetVersionID: c.DatasetVersionID, BasisRef: "SQL-only"}); e != nil {
						t.Error(e)
					}
				}
			}
			upload := datasetapp.NewUploadVersionService(o.tx, datasetinfra.NewPostgresRepository(pool), store).WithPublicationLease(journal)
			var version datasetdomain.DatasetVersion
			e := pins.ExecuteProcessing(readerContext(o), w, ex.ID, func(ctx context.Context) error {
				r, e := pins.ReadVersion(ctx, c.DatasetVersionID)
				if e != nil {
					return e
				}
				r.Close()
				version, e = upload.Handle(ctx, datasetapp.UploadVersionCommand{DatasetID: ex.OutputDatasetID, Filename: "output.csv", ContentType: "text/csv", Content: []byte("id,value\n1,controlled-output\n"), GeneratedByExecutionID: &ex.ID})
				return e
			})
			if store.puts != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_processing_output_intent`) != 1 {
				t.Fatal("actual output was not preceded by a typed intent", e, "puts", store.puts, "intents", count(t, pool, `SELECT count(*) FROM lifecycle_processing_output_intent`))
			}
			if mode == "current" {
				if e != nil || version.Status != datasetdomain.VersionReady || count(t, pool, `SELECT count(*) FROM lifecycle_processing_output_ready`) != 1 {
					t.Fatal("verified output failed publication", e)
				}
			} else {
				if !errors.Is(e, domain.ErrUseBlocked) || count(t, pool, `SELECT count(*) FROM lifecycle_processing_output_ready`) != 0 || count(t, pool, `SELECT count(*) FROM dataset_version WHERE status='READY' AND generated_by_execution_id IS NOT NULL`) != 0 {
					t.Fatal("late restriction published output", e)
				}
			}
		})
	}
}
