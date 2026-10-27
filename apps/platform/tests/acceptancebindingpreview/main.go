// One-time synthetic acceptance preparation. Never used by an API or CI worker.
// No domain command, external provider, model, execution or dispatch is invoked.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"log"
	"os"
	"path/filepath"
	"time"
)

func id(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("86000000-0000-4000-8000-%012d", n)) }
func main() {
	if os.Getenv("ACCEPTANCE_BINDING_PREPARE_ACK") != "ISOLATED_SYNTHETIC_ONLY" {
		log.Fatal("explicit synthetic preparation required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("READONLY_PREPARE_DSN"))
	must(err)
	if cfg.ConnConfig.Database != "agent_q2_acceptance_20261024" {
		log.Fatal("independent acceptance database required")
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	must(err)
	defer db.Close()
	root, err := os.Getwd()
	must(err)
	must(migration.NewRunner(db, filepath.Join(root, "migrations")).Up(ctx))
	tx, err := db.Begin(ctx)
	must(err)
	defer tx.Rollback(ctx)
	var count int
	must(tx.QueryRow(ctx, `SELECT count(*) FROM acceptance_party_representation`).Scan(&count))
	if count != 0 {
		log.Fatal("preparation already exists; never replay or replace facts")
	}
	ws := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	actor := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	version := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	standard := uuid.MustParse("82828282-8282-4282-8282-828282828282")
	var source string
	var raw []byte
	must(tx.QueryRow(ctx, `SELECT source_sha256,document FROM contract_version WHERE id=$1`, standard).Scan(&source, &raw))
	var doc map[string]any
	must(json.Unmarshal(raw, &doc))
	criteria := domain.ReadAcceptanceSource(doc)
	if criteria.Status != "SUPPORTED" {
		log.Fatal("explicit supported fixture source required")
	}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	for scenario, name := range []string{"current", "expired", "revoked", "ambiguous", "conflict", "mismatch", "replaced"} {
		delivery := uuid.MustParse("40404040-4040-4040-8040-404040404040")
		if scenario > 0 {
			delivery = id(scenario)
			_, err = tx.Exec(ctx, `INSERT INTO delivery_operation(id,workspace_id,dataset_version_id,idempotency_key,provider_name,provider_request_key,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,requested_expires_at,terminal_reason) VALUES($1,$2,$3,$4,'SYNTHETIC_NO_PROVIDER',$4,'BLOCKED','BLOCKED','SYNTHETIC_ACCEPTANCE','SYNTHETIC_ACCEPTANCE','SYNTHETIC_HISTORY','READ','SYNTHETIC_ONLY','API','DIRECT_DATA',now()+interval '1 hour','SYNTHETIC_ACCEPTANCE_FIXTURE')`, delivery, ws, version, "synthetic-binding-"+name)
			must(err)
		}
		basis, err := evidence.Append(ctx, tx, evidence.Record{ID: id(10000 + scenario), WorkspaceID: ws, EvidenceType: "SYNTHETIC_BUSINESS_BINDING", Title: "合成责任与标准适用依据 · " + name, SourceType: "SYNTHETIC_ACCEPTANCE", SourceID: &delivery, Metadata: map[string]any{"syntheticAcceptance": true, "scenario": name, "notEnterpriseAuthority": true}, CreatedBy: &actor}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: delivery, RelationType: "SYNTHETIC_BINDING_BASIS"})
		must(err)
		for i, role := range domain.AcceptanceRoles {
			repID, roleID := id(100+scenario*100+i), id(110+scenario*100+i)
			person := id(1000 + scenario*100 + i)
			subject := fmt.Sprintf("synthetic-%s-%s", name, role)
			party := "SYNTHETIC_ACCEPTANCE"
			if i == 0 {
				party = "SYNTHETIC_PROVIDER"
			}
			if scenario == 0 && i == 0 {
				person = actor
				subject = "agent-q2-operator"
			}
			if name == "conflict" && i == 3 {
				person = id(1000 + scenario*100)
			}
			var until *time.Time
			if name == "expired" && i == 1 {
				until = &end
			}
			_, err = tx.Exec(ctx, `INSERT INTO acceptance_party_representation(id,workspace_id,actor_id,principal_subject,party_ref,basis_evidence_id,valid_from,valid_to,origin,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'SYNTHETIC_FIXTURE',$9)`, repID, ws, person, subject, party, basis.ID, start, until, actor)
			must(err)
			_, err = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'SYNTHETIC_FIXTURE',$9)`, roleID, ws, delivery, version, role, repID, basis.ID, start, actor)
			must(err)
			if name == "revoked" && i == 2 {
				_, err = tx.Exec(ctx, `INSERT INTO acceptance_binding_disposition(id,workspace_id,representation_id,disposition,effective_at,reason,basis_evidence_id,recorded_by) VALUES($1,$2,$3,'REVOKED',$4,'合成验收：撤销确认人的代表资格',$5,$6)`, id(190+scenario*100), ws, repID, now, basis.ID, actor)
				must(err)
			}
			if name == "ambiguous" && i == 3 {
				_, err = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by) SELECT $1,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by FROM acceptance_responsibility_binding WHERE id=$2`, id(191+scenario*100), roleID)
				must(err)
			}
		}
		setHash := criteria.SetHash
		if name == "mismatch" {
			setHash = fmt.Sprintf("%064d", 0)
		}
		_, err = tx.Exec(ctx, `INSERT INTO acceptance_standard_applicability(id,workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,source_sha256,criteria_set_hash,confirmer_binding_id,basis_evidence_id,valid_from,origin,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'SYNTHETIC_FIXTURE',$11)`, id(120+scenario*100), ws, delivery, version, standard, source, setHash, id(112+scenario*100), basis.ID, start, actor)
		must(err)
		if name == "replaced" {
			_, err = tx.Exec(ctx, `INSERT INTO acceptance_binding_disposition(id,workspace_id,responsibility_id,disposition,effective_at,reason,basis_evidence_id,recorded_by) VALUES($1,$2,$3,'SUPERSEDED',$4,'合成验收：标准确认责任替换，旧适用确认不得沿用',$5,$6)`, id(192+scenario*100), ws, id(112+scenario*100), now, basis.ID, actor)
			must(err)
			_, err = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by) SELECT $1,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,origin,recorded_by FROM acceptance_responsibility_binding WHERE id=$2`, id(193+scenario*100), id(112+scenario*100))
			must(err)
		}
		if scenario == 0 {
			for n := 0; n < 16; n++ {
				_, err = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by,created_at) VALUES($1,$2,$3,$4,'RECEIVER',$5,$6,$7,$8,'SYNTHETIC_FIXTURE',$9,$7)`, id(2000+n), ws, delivery, version, id(101), basis.ID, start, end, actor)
				must(err)
			}
		}
	}
	_, err = tx.Exec(ctx, `GRANT SELECT ON acceptance_party_representation,acceptance_responsibility_binding,acceptance_standard_applicability,acceptance_binding_disposition TO agent_q2_reader_20261024`)
	must(err)
	must(tx.Commit(ctx))
	log.Print("Seven SYNTHETIC_FIXTURE scenarios prepared in independent DB; SELECT-only grants; no business command or provider invoked")
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
