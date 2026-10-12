package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

// Real PostgreSQL protocol tests; this port is SQL-only, not native acceptance.
var readerTestBytes = []byte("synthetic-reader-bytes\n")

type readerTestPort struct {
	journal            PinnedReaderCallJournal
	calls              atomic.Int64
	before             func(domain.ReaderPin)
	crash, unknownStop bool
}

func (p *readerTestPort) ReadExactPinned(ctx context.Context, c domain.Copy, pin domain.ReaderPin) (PinnedReadResult, error) {
	if p.before != nil {
		p.before(pin)
	}
	r := PinnedReadResult{Copy: c, PinID: pin.ID, CallID: uuid.New()}
	e := p.journal.StartStorageCall(ctx, StorageCallStart{ID: r.CallID, Copy: c, Operation: StorageReadPinned, StartedAt: time.Now().UTC(), ReaderPin: &pin})
	if e != nil {
		r.StoppedAt = time.Now().UTC()
		return r, e
	}
	p.calls.Add(1)
	at := time.Now().UTC()
	e = p.journal.ObserveStorageCall(ctx, StorageCallObservation{CallID: r.CallID, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: c.ContentSHA256})
	if p.crash {
		panic("SQL-only crash after observed read, before owner stop")
	}
	r.Bytes = readerTestBytes
	if !p.unknownStop {
		r.StoppedAt = time.Now().UTC()
	}
	return r, e
}
func newReaderFixture(t *testing.T) (*pgxpool.Pool, uuid.UUID, domain.Copy, *OccupancyService, *FixtureReaderService, *readerTestPort, domain.Plan) {
	pool := planDatabase(t)
	w, cs, o, a, _, _, _ := occupancyFixture(t, pool)
	ctx := context.Background()
	var ns, dataset uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT c.namespace_id,v.dataset_id FROM lifecycle_fixture_copy c JOIN dataset_version v ON v.id=c.dataset_version_id WHERE c.id=$1`, cs[0].ID).Scan(&ns, &dataset); e != nil {
		t.Fatal(e)
	}
	version := uuid.New()
	copy := cs[0]
	copy.ID = uuid.New()
	copy.DatasetVersionID = version
	copy.Key = "sql-only/reader.csv"
	copy.ProviderVersion = "sql-only-reader-version"
	copy.ByteSize = int64(len(readerTestBytes))
	sum := sha256.Sum256(readerTestBytes)
	copy.ContentSHA256 = hex.EncodeToString(sum[:])
	if _, e := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,2,'READY','OBJECT_STORAGE','s3://no-provider-configured/reader.csv','SHA256',$3,$4)`, version, dataset, copy.ContentSHA256, copy.ByteSize); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, copy.ID, w, ns, version, copy.Key, copy.ProviderVersion, copy.ContentSHA256, copy.ByteSize); e != nil {
		t.Fatal(e)
	}
	a.caller.Capabilities[CapabilityPreparePlan] = struct{}{}
	plan, e := NewPlanService(o.tx, o.repo, &testAuthority{caller: a.caller}).PrepareDeletePlan(ctx, PrepareDeletePlanCommand{WorkspaceID: w, CopyIDs: []uuid.UUID{copy.ID}, IdempotencyKey: "reader-plan"})
	if e != nil {
		t.Fatal(e)
	}
	o.termination = nil // compose a distinct real owner, do not reuse SQL verifier
	p := &readerTestPort{journal: PinnedReaderCallJournal{Service: o, Observations: NewPostgresStorageCallJournal(o)}}
	s := NewFixtureReaderService(o, p)
	if s == nil {
		t.Fatal("reader owner unavailable")
	}
	return pool, w, copy, o, s, p, plan
}

func readerContext(o *OccupancyService) context.Context {
	return WithFixtureReaderCaller(context.Background(), o.authority.(*occupancyAuthority).caller)
}

type blockingReaderWriter struct {
	entered, release chan struct{}
	fail             bool
}

type flushingReaderWriter struct{ blockingReaderWriter }

func (w *flushingReaderWriter) WriteContext(ctx context.Context, b []byte) (int, error) {
	// Write can buffer successfully, but its output owner still has a flush.
	close(w.entered)
	<-w.release
	return 0, ctx.Err()
}

func TestFixtureReaderWaitsForBufferedOutputFlushOwner(t *testing.T) {
	pool, w, c, o, s, _, plan := newReaderFixture(t)
	ctx, cancel := context.WithCancel(readerContext(o))
	defer cancel()
	writer := &flushingReaderWriter{blockingReaderWriter{entered: make(chan struct{}), release: make(chan struct{})}}
	done := make(chan error, 1)
	go func() {
		_, e := s.ReadTo(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "buffered-output-flush"}, CopyID: c.ID, ReaderID: uuid.New()}, writer)
		done <- e
	}()
	<-writer.entered
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination`) != 0 {
		t.Fatal("buffered write released pin before flush")
	}
	cancel()
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
		t.Fatal("cancel released pending flush")
	}
	close(writer.release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 {
		t.Fatal("confirmed flush stop failed to release owner pin")
	}
}

func (w *blockingReaderWriter) Write(b []byte) (int, error) {
	close(w.entered)
	<-w.release
	if w.fail {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}
func TestFixtureReaderPinSurvivesRestrictionAndCancelledWriterUntilStopped(t *testing.T) {
	pool, w, c, o, s, p, plan := newReaderFixture(t)
	ctx, cancel := context.WithCancel(readerContext(o))
	defer cancel()
	writer := &blockingReaderWriter{entered: make(chan struct{}), release: make(chan struct{}), fail: true}
	cmd := AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "actual-read"}, CopyID: c.ID, ReaderID: uuid.New()}
	done := make(chan error, 1)
	go func() { _, e := s.ReadTo(ctx, cmd, writer); done <- e }()
	<-writer.entered
	if p.calls.Load() != 1 || occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination`) != 0 {
		t.Fatal("pin missing while output is active")
	}
	if _, e := o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: CommandHeader{w, "during-read-restriction"}, DatasetVersionID: c.DatasetVersionID, BasisRef: "controlled-sql-only"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ReadTo(readerContext(o), AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "blocked-new-read"}, CopyID: c.ID, ReaderID: uuid.New()}, io.Discard); e == nil || p.calls.Load() != 1 {
		t.Fatal("restriction admitted new bytes")
	}
	cancel()
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 1 {
		t.Fatal("cancellation released still running writer")
	}
	close(writer.release)
	if e := <-done; !errors.Is(e, io.ErrClosedPipe) {
		t.Fatal(e)
	}
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination WHERE outcome='CANCELLATION_CONFIRMED'`) != 1 {
		t.Fatal("positive owner stop did not terminate pin")
	}
	if count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 1 {
		t.Fatal("read invocation cost lost")
	}
}
func TestFixtureReaderCompleteReplayAndCrashedOwnerNeverReleaseByLease(t *testing.T) {
	for _, scenario := range []string{"complete", "crash", "unknown-stop"} {
		t.Run(scenario, func(t *testing.T) {
			pool, w, c, o, s, p, plan := newReaderFixture(t)
			cmd := AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "single-read"}, CopyID: c.ID, ReaderID: uuid.New()}
			p.crash = scenario == "crash"
			p.unknownStop = scenario == "unknown-stop"
			if p.crash {
				func() {
					defer func() {
						if recover() == nil {
							t.Fatal("crash not reached")
						}
					}()
					_, _ = s.ReadTo(readerContext(o), cmd, io.Discard)
				}()
			} else {
				var dst bytes.Buffer
				r, e := s.ReadTo(readerContext(o), cmd, &dst)
				if scenario == "complete" && (e != nil || !r.StopConfirmed || !bytes.Equal(dst.Bytes(), readerTestBytes)) {
					t.Fatal(r, e)
				}
				if scenario == "unknown-stop" && !errors.Is(e, domain.ErrTerminationUnknown) {
					t.Fatal(e)
				}
			}
			before := p.calls.Load()
			var replay bytes.Buffer
			r, e := s.ReadTo(readerContext(o), cmd, &replay)
			if e != nil || !r.Admission.Replayed || r.Admission.Admitted || replay.Len() != 0 || p.calls.Load() != before {
				t.Fatal("historical admission replay reopened native bytes", e)
			}
			expected := int64(1)
			if scenario == "complete" {
				expected = 0
			}
			if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != expected {
				t.Fatal("unconfirmed owner pin released")
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'`) != 1 {
				t.Fatal("native read repeated")
			}
		})
	}
}
func TestPinnedPhysicalStartRechecksRestrictionAndRejectsForgedGeneration(t *testing.T) {
	pool, w, c, o, s, p, plan := newReaderFixture(t)
	p.before = func(pin domain.ReaderPin) {
		forged := pin
		forged.Generation++
		if e := p.journal.StartStorageCall(context.Background(), StorageCallStart{ID: uuid.New(), Copy: c, Operation: StorageReadPinned, StartedAt: time.Now().UTC(), ReaderPin: &forged}); e == nil {
			t.Fatal("forged generation admitted")
		}
		if _, e := o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: CommandHeader{w, "between-admit-and-io"}, DatasetVersionID: c.DatasetVersionID, BasisRef: "controlled-sql-only"}); e != nil {
			t.Fatal(e)
		}
	}
	r, e := s.ReadTo(readerContext(o), AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "fresh-physical-gate"}, CopyID: c.ID, ReaderID: uuid.New()}, io.Discard)
	if e == nil || p.calls.Load() != 0 || !r.StopConfirmed || count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'`) != 0 || occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 {
		t.Fatal("old pin admitted I/O or failed confirmed no-send stop", r, e)
	}
}

func TestFixtureReaderBindsAuthenticatedTransportCallerToCurrentAuthority(t *testing.T) {
	pool, w, c, o, s, port, _ := newReaderFixture(t)
	cmd := AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "caller-binding"}, CopyID: c.ID, ReaderID: uuid.New()}
	if _, e := s.ReadTo(context.Background(), cmd, io.Discard); !errors.Is(e, principal.ErrUnauthenticated) {
		t.Fatal("missing trusted transport principal admitted", e)
	}
	caller := o.authority.(*occupancyAuthority).caller
	caller.ActorID = uuid.New()
	if _, e := s.ReadTo(WithFixtureReaderCaller(context.Background(), caller), cmd, io.Discard); !errors.Is(e, principal.ErrCapabilityDenied) {
		t.Fatal("transport principal switched to configured owner", e)
	}
	if port.calls.Load() != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_pin`) != 0 {
		t.Fatal("rejected caller created a pin or native read")
	}
}
