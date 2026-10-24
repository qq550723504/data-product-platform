// Explicit new-DB fixture preparation. No business Command or provider runs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/tests/acceptancefixture"
	"log"
	"os"
	"path/filepath"
)

func id(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("74000000-0000-4000-8000-%012d", n)) }

type person struct{ Actor, Subject string }

func main() {
	if os.Getenv("ACCEPTANCE_IDENTITY_PREPARE_ACK") != "NEW_ISOLATED_1A_ONLY" {
		log.Fatal("explicit new isolated preparation required")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(os.Getenv("ACCEPTANCE_IDENTITY_PREPARE_DSN"))
	must(e)
	if cfg.ConnConfig.Database != "acceptance_1a_20261024" {
		log.Fatal("exact independent 1A database required")
	}
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	must(e)
	defer db.Close()
	root, e := os.Getwd()
	must(e)
	var count int
	must(db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&count))
	if count != 0 {
		log.Fatal("existing database must not be prepared again")
	}
	must(migration.NewRunner(db, filepath.Join(root, "migrations")).Up(ctx))
	var config struct {
		Issuer string
		People map[string]person
	}
	raw, e := os.ReadFile(filepath.Join(root, ".data/acceptance-identity-1a/profile.json"))
	must(e)
	must(json.Unmarshal(raw, &config))
	tx, e := db.Begin(ctx)
	must(e)
	defer tx.Rollback(ctx)
	names := []string{"provider", "receiver", "confirmer", "reviewer", "archivist", "disposer"}
	for i, name := range names {
		p := config.People[name]
		actor := uuid.MustParse(p.Actor)
		_, e = tx.Exec(ctx, `INSERT INTO platform_actor_state(actor_id)VALUES($1);`, actor)
		must(e)
		_, e = tx.Exec(ctx, `INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '2 days')`, id(10+i), config.Issuer, p.Subject, actor)
		must(e)
	}
	for k, workspace := range []string{"11111111-1111-4111-8111-111111111111", "12121212-1212-4212-8212-121212121212"} {
		ws := uuid.MustParse(workspace)
		base := 1000 * (k + 1)
		dataset, version, delivery, contract, standard := id(base), id(base+1), id(base+2), id(base+3), id(base+4)
		for i, name := range names {
			if k == 1 && name == "provider" {
				continue
			}
			caps := []string{principal.CapabilityAcceptanceRead}
			if i == 0 {
				caps = append(caps, principal.CapabilityAcceptanceDraft)
			}
			if i == 1 {
				caps = append(caps, principal.CapabilityReceiptRecord)
			}
			if i == 2 {
				caps = append(caps, principal.CapabilityAcceptanceBasis)
			}
			_, e = tx.Exec(ctx, `INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '1 day')`, id(base+10+i), config.People[name].Actor, ws, caps)
			must(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,metadata)VALUES($1,$2,$3,$4,'RAW','{"syntheticAcceptance":true}')`, dataset, ws, fmt.Sprintf("IDENTITY-1A-%d", k), []string{"园区成果 · 1A 合成验收样本", "制造成果 · 1A 合成验收样本"}[k])
		must(e)
		_, e = tx.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,row_count,ready_at,metadata)VALUES($1,$2,1,'READY','fixture://no-payload','SHA256',repeat('a',64),1,now(),'{"syntheticAcceptance":true}')`, version, dataset)
		must(e)
		_, e = tx.Exec(ctx, `INSERT INTO delivery_operation(id,workspace_id,dataset_version_id,idempotency_key,provider_name,provider_request_key,status,current_gate_decision,principal_ref,effective_consumer_ref,purpose,action,scope_ref,delivery_channel,delivery_mode,requested_expires_at,terminal_reason)VALUES($1,$2,$3,$4,'SYNTHETIC_NO_PROVIDER',$4,'BLOCKED','BLOCKED','SYNTHETIC_ACCEPTANCE','SYNTHETIC_ACCEPTANCE','SYNTHETIC_HISTORY','READ','SYNTHETIC_ONLY','API','DIRECT_DATA',now()+interval '1 hour','SYNTHETIC_ACCEPTANCE_FIXTURE')`, delivery, ws, version, fmt.Sprintf("identity-1a-%d", k))
		must(e)
		doc := map[string]any{"metadata": map[string]any{"syntheticAcceptance": true}, "spec": map[string]any{"acceptanceCriteria": map[string]any{"version": "1", "criteria": []map[string]any{{"id": "delivery-received", "clause": "1.1", "text": "接收方核对收到成果并提供明确依据", "required": true, "method": "MANUAL_CHECK", "evidenceCategory": "RECEIPT_ATTESTATION"}, {"id": "traceability", "clause": "1.2", "text": "成果版本及责任依据可追溯", "required": true, "method": "EVIDENCE_ASSERTION", "evidenceCategory": "TRACEABILITY"}}}}}
		payload, e := json.Marshal(doc)
		must(e)
		hash := sha256.Sum256(payload)
		sourceHash := hex.EncodeToString(hash[:])
		criteria := domain.ReadAcceptanceSource(doc)
		if criteria.Status != "SUPPORTED" {
			log.Fatal("supported fixture criteria required")
		}
		_, e = tx.Exec(ctx, `INSERT INTO data_contract(id,workspace_id,code,name)VALUES($1,$2,$3,'1A 合成验收标准')`, contract, ws, fmt.Sprintf("IDENTITY-STANDARD-%d", k))
		must(e)
		_, e = tx.Exec(ctx, `INSERT INTO contract_version(id,contract_id,major_version,minor_version,patch_version,status,document,source_sha256,published_at)VALUES($1,$2,1,0,0,'PUBLISHED',$3,$4,now())`, standard, contract, payload, sourceHash)
		must(e)
		creator := uuid.MustParse(config.People["provider"].Actor)
		basis, e := evidence.Append(ctx, tx, evidence.Record{ID: id(base + 20), WorkspaceID: ws, EvidenceType: "SYNTHETIC_BUSINESS_BINDING", Title: "1A 合成责任/适用依据，不代表企业授权", SourceType: "SYNTHETIC_ACCEPTANCE", SourceID: &delivery, CreatedBy: &creator, Metadata: map[string]any{"syntheticAcceptance": true, "notEnterpriseAuthority": true}})
		must(e)
		for i, role := range domain.AcceptanceRoles {
			actor := uuid.MustParse(config.People[names[i]].Actor)
			party := "SYNTHETIC_ACCEPTANCE"
			if i == 0 {
				party = "SYNTHETIC_PROVIDER"
			}
			// The B-space provider has a role fact but no workspace grant, to test that
			// role/representation alone never grants identity admission.
			_, e = tx.Exec(ctx, `INSERT INTO acceptance_party_representation(id,workspace_id,actor_id,principal_subject,party_ref,basis_evidence_id,valid_from,valid_to,origin,recorded_by)VALUES($1,$2,$3,$4,$5,$6,now()-interval '1 hour',now()+interval '1 day','SYNTHETIC_FIXTURE',$3)`, id(base+30+i), ws, actor, actor.String(), party, basis.ID)
			must(e)
			_, e = tx.Exec(ctx, `INSERT INTO acceptance_responsibility_binding(id,workspace_id,delivery_operation_id,dataset_version_id,role,representation_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,now()-interval '1 hour',now()+interval '1 day','SYNTHETIC_FIXTURE',$8)`, id(base+40+i), ws, delivery, version, role, id(base+30+i), basis.ID, actor)
			must(e)
		}
		_, e = tx.Exec(ctx, `INSERT INTO acceptance_standard_applicability(id,workspace_id,delivery_operation_id,dataset_version_id,contract_version_id,source_sha256,criteria_set_hash,confirmer_binding_id,basis_evidence_id,valid_from,valid_to,origin,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now()-interval '1 hour',now()+interval '1 day','SYNTHETIC_FIXTURE',$10)`, id(base+50), ws, delivery, version, standard, sourceHash, criteria.SetHash, id(base+42), basis.ID, config.People["confirmer"].Actor)
		must(e)
	}
	must(acceptancefixture.AppendBasisMaterials(ctx, tx))
	must(tx.Commit(ctx))
	fmt.Println("Schema76 and six synthetic directory people/two workspaces prepared. No business Command/provider/execution invoked.")
}
func must(e error) {
	if e != nil {
		_ = os.WriteFile(".data/acceptance-identity-1a/prepare-error.log", []byte(e.Error()), 0600)
		log.Fatal("isolated fixture preparation failed; inspect protected local error log")
	}
}
