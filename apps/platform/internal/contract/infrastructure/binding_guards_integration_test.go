package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"os"
	"strings"
	"testing"
	"time"
)

// Always runs in PostgreSQL CI without relying on retained local preview IDs.
// All own test facts, successful probes and rejected mutations roll back.
func TestAcceptanceBindingGuardsPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	ws, other, actor, dataset, version, delivery, contract, standard, rep, role, app := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'synthetic binding guards','RAW')`, dataset, ws, dataset.String())
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,row_count,ready_at) VALUES($1,$2,1,'READY','synthetic://no-provider','SHA256',repeat('a',64),1,now())`, version, dataset)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO delivery_operation(id,workspace_id,dataset_version_id,idempotency_key,provider_name,provider_request_key,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,requested_expires_at,terminal_reason) VALUES($1,$2,$3,$4,'SYNTHETIC_NO_PROVIDER',$4,'BLOCKED','BLOCKED','synthetic-principal','synthetic-receiver','SYNTHETIC_HISTORY','READ','SYNTHETIC_ONLY','API','DIRECT_DATA',now()+interval '1 hour','SYNTHETIC_ACCEPTANCE_FIXTURE')`, delivery, ws, version, delivery.String())
	must(err)
	b, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: ws, EvidenceType: "SYNTHETIC_BUSINESS_BINDING", Title: "rollback fixture", SourceID: &delivery})
	must(err)
	foreign, err := evidence.Append(ctx, tx, evidence.Record{WorkspaceID: other, EvidenceType: "SYNTHETIC_BUSINESS_BINDING"})
	must(err)
	doc := map[string]any{"spec": map[string]any{"acceptanceCriteria": map[string]any{"version": "1", "criteria": []any{map[string]any{"id": "C1", "clause": "1", "text": "synthetic criterion", "required": true, "method": "MANUAL_CHECK", "evidenceCategory": "RECEIPT"}}}}}
	raw, err := json.Marshal(doc)
	must(err)
	criteria := domain.ReadAcceptanceSource(doc)
	_, err = tx.Exec(ctx, `INSERT INTO data_contract(id,workspace_id,code,name) VALUES($1,$2,$3,'synthetic binding source')`, contract, ws, contract.String())
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO contract_version(id,contract_id,major_version,minor_version,patch_version,status,document,source_sha256,published_at) VALUES($1,$2,1,0,0,'PUBLISHED',$3,repeat('a',64),now())`, standard, contract, raw)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO acceptance_party_representation(id,workspace_id,actor_id,principal_subject,party_ref,basis_evidence_id,valid_from,origin,recorded_by) VALUES($1,$2,$3,'synthetic-subject','synthetic-receiver',$4,now()-interval '1 hour','SYNTHETIC_FIXTURE',$3)`, rep, ws, actor, b.ID)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by) VALUES($1,$2,$3,$4,'BASIS_CONFIRMER',$5,$6,now()-interval '1 hour','SYNTHETIC_FIXTURE',$7)`, role, ws, delivery, version, rep, b.ID, actor)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO acceptance_standard_applicability(id,workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,source_sha256,criteria_set_hash,confirmer_binding_id,basis_evidence_id,valid_from,origin,recorded_by) VALUES($1,$2,$3,$4,$5,repeat('a',64),$6,$7,$8,now()-interval '1 hour','SYNTHETIC_FIXTURE',$9)`, app, ws, delivery, version, standard, criteria.SetHash, role, b.ID, actor)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO acceptance_binding_disposition(id,workspace_id,representation_id,disposition,effective_at,reason,basis_evidence_id,recorded_by) VALUES($1,$2,$3,'REVOKED',now()+interval '1 hour','scheduled test disposition',$4,$5)`, uuid.New(), ws, rep, b.ID, actor)
	must(err)
	facts, err := readBindingFacts(ctx, tx, q.Query{WorkspaceID: ws, DeliveryID: &delivery, DatasetVersionID: &version})
	must(err)
	if len(facts) != 4 {
		t.Fatalf("scoped facts %d", len(facts))
	}
	var at time.Time
	must(tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&at))
	bc := domain.AcceptanceBindingContext{WorkspaceID: ws.String(), DeliveryID: delivery.String(), DatasetVersionID: version.String(), ConsumerRef: "synthetic-receiver", ContractVersionID: standard.String(), SourceSHA256: strings.Repeat("a", 64), CriteriaSetHash: criteria.SetHash, StandardSupported: true, At: at}
	rs, a, h := domain.EvaluateAcceptanceBindings(bc, facts)
	if a.Status != "MATCHED" || rs[2].Status != "ASSIGNED" {
		t.Fatalf("uncommitted typed facts not readable: %+v %+v", rs, a)
	}
	bc.At = at.Add(2 * time.Hour)
	rs, a, _ = domain.EvaluateAcceptanceBindings(bc, facts)
	if rs[2].Status != "INACTIVE" || a.Status != "MISMATCH" {
		t.Fatal("scheduled disposition did not block at effective time")
	}
	if len(h) != 4 {
		t.Fatal("history lost")
	}
	missing, err := readBindingFacts(ctx, tx, q.Query{WorkspaceID: other, DeliveryID: &delivery, DatasetVersionID: &version})
	must(err)
	if len(missing) != 0 {
		t.Fatal("foreign facts leaked")
	}
	reject := func(sql, reason string, args ...any) {
		t.Helper()
		_, e := tx.Exec(ctx, `SAVEPOINT rejected_probe`)
		must(e)
		_, e = tx.Exec(ctx, sql, args...)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "23514" || !strings.Contains(pe.Message, reason) {
			t.Fatalf("expected %s, got %v", reason, e)
		}
		_, e = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT rejected_probe`)
		must(e)
	}
	for _, table := range []string{"acceptance_party_representation", "acceptance_responsibility_binding", "acceptance_standard_applicability", "acceptance_binding_disposition"} {
		reject("UPDATE "+table+" SET recorded_by=recorded_by", "immutable")
		reject("DELETE FROM "+table, "immutable")
	}
	reject(`INSERT INTO acceptance_responsibility_binding SELECT $1,$2,delivery_operation_id,dataset_version_id,role,representation_id,$3,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_responsibility_binding WHERE id=$4`, "acceptance target crosses scope", uuid.New(), other, foreign.ID, role)
	reject(`INSERT INTO acceptance_standard_applicability SELECT $1,workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,repeat('b',64),criteria_set_hash,confirmer_binding_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_standard_applicability WHERE id=$2`, "source crosses scope/hash", uuid.New(), app)
	reject(`INSERT INTO acceptance_party_representation SELECT $1,workspace_id,actor_id,principal_subject,party_ref,$2,valid_from,valid_to,origin,recorded_by,created_at FROM acceptance_party_representation WHERE id=$3`, "evidence crosses workspace", uuid.New(), foreign.ID, rep)
	// Check the actual installed admission trigger, not a naming convention only.
	var admission int
	must(tx.QueryRow(ctx, `SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_proc p ON p.oid=t.tgfoid WHERE c.relname IN ('acceptance_responsibility_binding','acceptance_standard_applicability') AND p.proname='lifecycle_reference_admission_guard' AND t.tgenabled='O'`).Scan(&admission))
	if admission != 2 {
		t.Fatal("lifecycle reference admission missing")
	}
}
