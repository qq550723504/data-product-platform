package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
)

func seedCustody(t *testing.T, pool *pgxpool.Pool) (CustodyPlan, []ld.Copy) {
	t.Helper()
	ctx := context.Background()
	p := CustodyPlan{WorkspaceID: uuid.New(), ActorID: uuid.New(), RunID: "synthetic-retained-run", PrincipalRef: "trusted-synthetic-operator", ProviderWitness: "SQL_ONLY_NO_NATIVE_PROVIDER", Endpoint: "127.0.0.1:1", OutputDirectory: "synthetic-private-not-created", PlanSHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)}
	var copies []ld.Copy
	for i, kind := range []string{"STANDARDIZED", "CURATED"} {
		dataset, version, namespace, provider, incarnation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		body := []byte(fmt.Sprintf("id,value\n%d,unchanged\n", i))
		hash := sha256.Sum256(body)
		target := CustodyTarget{VersionID: version, URI: "s3://raw/datasets/" + version.String() + ".csv", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(body))}
		if i == 0 {
			p.First = target
		} else {
			p.Second = target
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Synthetic custody',$4)`, dataset, p.WorkspaceID, uuid.NewString(), kind); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5)`, version, dataset, target.URI, target.SHA256, target.Bytes); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace) VALUES($1,$2,$3,$4,'sql-only-custody')`, namespace, p.WorkspaceID, provider, incarnation); err != nil {
			t.Fatal(err)
		}
		c := ld.Copy{ID: uuid.New(), WorkspaceID: p.WorkspaceID, DatasetVersionID: version, ProviderInstanceID: provider, IncarnationID: incarnation, Namespace: "sql-only-custody", Key: target.Key(), ProviderVersion: "sql-only-not-a-real-object", ContentSHA256: target.SHA256, ByteSize: target.Bytes}
		if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.ID, c.WorkspaceID, namespace, version, c.Key, c.ProviderVersion, c.ContentSHA256, c.ByteSize); err != nil {
			t.Fatal(err)
		}
		copies = append(copies, c)
	}
	return p, copies
}
func custodyObservation(call uuid.UUID, target CustodyTarget) CustodyObservation {
	at := time.Now().UTC().Truncate(time.Microsecond)
	return CustodyObservation{CallID: call, InvokedAt: at, ObservedAt: at, InvocationCertainty: "CONFIRMED", HTTPStatus: 200, Bytes: target.Bytes, SHA256: target.SHA256, EOF: true, CloseOK: true}
}
func assertCustodyReference(t *testing.T, pool *pgxpool.Pool, c ld.Copy, want int) {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, refs, err := InspectFixtureReferences(context.Background(), tx, c)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range refs {
		if r.Table == "operator_custody_acquisition" && r.Count != 0 {
			found = true
			if r.Unreleased != want {
				t.Fatalf("custody reference %+v want %d", r, want)
			}
		}
	}
	if !found {
		t.Fatal("custody FK was not inspected")
	}
}
func TestOperatorCustodyAttemptsCostFactsReplayAndExplicitTermination(t *testing.T) {
	pool := planDatabase(t)
	p, copies := seedCustody(t, pool)
	ctx := context.Background()
	s, _ := NewOperatorCustody(pool, p)
	if replay, err := s.Prepare(ctx, "one"); err != nil || replay {
		t.Fatal("prepare", err)
	}
	assertCustodyReference(t, pool, copies[0], 1)
	other, _ := NewOperatorCustody(pool, p)
	if _, err := other.Prepare(ctx, "one"); !errors.Is(err, ErrCustodyInterrupted) {
		t.Fatal("incomplete replay readmission", err)
	}
	id, err := s.StartCustodyCall(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(ctx, false, ""); err == nil {
		t.Fatal("start-only UNKNOWN released pin")
	}
	o := custodyObservation(id, p.First)
	o.HTTPStatus = 503
	o.Failure = "HTTP"
	o.SHA256 = ""
	o.Bytes = 0
	o.EOF = false
	if err = s.ObserveCustodyCall(ctx, o); err != nil {
		t.Fatal(err)
	}
	if err = s.ObserveCustodyCall(ctx, o); err != nil {
		t.Fatal("same observation replay", err)
	}
	changed := o
	changed.Failure = "TRANSPORT"
	if err = s.ObserveCustodyCall(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("observation rewritten", err)
	}
	if _, err = s.StartCustodyCall(ctx, 1); err == nil {
		t.Fatal("same member physical retry")
	}
	if n := count(t, pool, `SELECT count(*) FROM cost_event e JOIN cost_allocation a ON a.cost_event_id=e.id WHERE a.operator_custody_call_id=$1 AND e.activity_id=$1 AND e.workspace_id=$2 AND e.quantity=1 AND e.unit='invocation' AND e.cost_type='OPERATOR_CUSTODY_INVOCATION' AND e.amount IS NULL`, id, p.WorkspaceID); n != 1 {
		t.Fatal("failed physical call cost", n)
	}
	for _, table := range []string{"audit_event", "outbox_event", "evidence"} {
		column, value := "action", "OperatorCustodyCallObserved"
		if table == "outbox_event" {
			column = "event_type"
		}
		if table == "evidence" {
			column = "title"
		}
		if n := count(t, pool, "SELECT count(*) FROM "+table+" WHERE "+column+"=$1", value); n != 1 {
			t.Fatalf("atomic %s facts=%d", table, n)
		}
	}
	if err = s.Finish(ctx, true, strings.Repeat("c", 64)); err == nil {
		t.Fatal("failed/partial call accepted")
	}
	if err = s.Finish(ctx, false, ""); err != nil {
		t.Fatal(err)
	}
	assertCustodyReference(t, pool, copies[0], 0)
	if _, err = s.StartCustodyCall(ctx, 2); err == nil {
		t.Fatal("terminated acquisition readmitted")
	}
	for _, table := range []string{"operator_custody_acquisition", "operator_custody_call", "operator_custody_observation", "operator_custody_termination"} {
		for _, sql := range []string{"UPDATE " + table + " SET workspace_id=workspace_id", "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err = pool.Exec(ctx, sql); err == nil {
				t.Fatal("history mutation", sql)
			}
		}
	}
}

func TestOperatorCustodyWorkspaceAndFreshFenceAdmission(t *testing.T) {
	pool := planDatabase(t)
	p, _ := seedCustody(t, pool)
	ctx := context.Background()
	foreign := p
	foreign.WorkspaceID = uuid.New()
	s, _ := NewOperatorCustody(pool, foreign)
	if _, err := s.Prepare(ctx, "foreign"); err == nil {
		t.Fatal("foreign workspace admitted")
	}
	s, _ = NewOperatorCustody(pool, p)
	if _, err := s.Prepare(ctx, "fenced"); err != nil {
		t.Fatal(err)
	}
	block, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback(context.Background())
	rev, err := deliveryfence.Advance(ctx, block, p.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = block.Exec(ctx, `INSERT INTO lifecycle_content_use_restriction(id,workspace_id,dataset_version_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at) VALUES($1,$2,$3,'synthetic-stop',$4,$5,$6,clock_timestamp())`, uuid.New(), p.WorkspaceID, p.First.VersionID, p.ActorID, p.PrincipalRef, rev); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.StartCustodyCall(ctx, 1); done <- err }()
	select {
	case err := <-done:
		t.Fatal("call crossed held admission fence", err)
	case <-time.After(80 * time.Millisecond):
	}
	if err = block.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("post-restriction call admitted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call did not finish")
	}
	if n := count(t, pool, `SELECT count(*) FROM operator_custody_call`); n != 0 {
		t.Fatal("denied call recorded as physical attempt")
	}
	if err = s.Finish(ctx, false, ""); err != nil {
		t.Fatal("failed owner could not close after restriction", err)
	}
}

func TestOperatorCustodySuccessfulPairCannotLoseTypedCostOrChangeKey(t *testing.T) {
	pool := planDatabase(t)
	p, _ := seedCustody(t, pool)
	ctx := context.Background()
	s, _ := NewOperatorCustody(pool, p)
	if _, err := s.Prepare(ctx, "pair"); err != nil {
		t.Fatal(err)
	}
	for i, target := range []CustodyTarget{p.First, p.Second} {
		id, err := s.StartCustodyCall(ctx, i+1)
		if err != nil {
			t.Fatal(err)
		}
		o := custodyObservation(id, target)
		if i == 1 {
			o.InvocationCertainty = "UNKNOWN"
			o.Failure = "TRANSPORT"
			o.HTTPStatus = 0
			o.EOF = false
			o.CloseOK = false
			o.SHA256 = ""
		}
		if err = s.ObserveCustodyCall(ctx, o); err != nil {
			t.Fatal(err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM cost_allocation WHERE operator_custody_call_id IS NOT NULL`); n != 2 {
		t.Fatal("UNKNOWN attempt lost cost")
	}
	if err := s.Finish(ctx, true, strings.Repeat("d", 64)); err == nil {
		t.Fatal("UNKNOWN pair accepted")
	}
	changed := p
	changed.ActorID = uuid.New()
	other, _ := NewOperatorCustody(pool, changed)
	if _, err := other.Prepare(ctx, "pair"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("key changed operator identity", err)
	}
}

func TestOperatorCustodyMigration71ReplayAndCostScopeGuards(t *testing.T) {
	pool, frozen := planDatabaseAtVersion(t, 70)
	ctx := context.Background()
	_, file, _, _ := runtime.Caller(0)
	// Freeze this historical 70 -> 71 replay/down contract at migration 71.
	// Later additive migrations are verified by current-schema tests instead.
	for _, name := range []string{"000071_operator_custody.up.sql", "000071_operator_custody.down.sql"} {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(frozen, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	runner := migration.NewRunner(pool, frozen)
	for i := 0; i < 2; i++ {
		if err := runner.Up(ctx); err != nil {
			t.Fatal("70 -> 71 -> 71 replay", err)
		}
	}
	var version int
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&version); err != nil || version != 71 {
		t.Fatal("migration head", version, err)
	}
	p, _ := seedCustody(t, pool)
	s, _ := NewOperatorCustody(pool, p)
	if _, err := s.Prepare(ctx, "cost-guard"); err != nil {
		t.Fatal(err)
	}
	call, err := s.StartCustodyCall(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Even same-workspace cost cannot point at an unobserved/invented physical
	// invocation. A cross-workspace activity cannot use the new typed subject.
	for _, workspace := range []uuid.UUID{p.WorkspaceID, uuid.New()} {
		event := uuid.New()
		if _, err = pool.Exec(ctx, `INSERT INTO cost_event(id,workspace_id,activity_id,cost_type,quantity,unit,pricing_mode,occurred_at) VALUES($1,$2,$3,'OPERATOR_CUSTODY_INVOCATION',1,'invocation','ACTUAL',clock_timestamp())`, event, workspace, call); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,operator_custody_call_id) VALUES($1,$2,$3)`, uuid.New(), event, call); err == nil {
			t.Fatal("forged/cross-workspace call cost admitted")
		}
	}
	if err = runner.Down(ctx); err == nil {
		t.Fatal("down removed immutable acquisition history")
	}
}
