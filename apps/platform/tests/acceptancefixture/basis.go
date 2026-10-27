// Package acceptancefixture contains explicitly synthetic isolated test data.
package acceptancefixture

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"strings"
	"time"
)

func ID(n int) uuid.UUID { return uuid.MustParse(fmt.Sprintf("74000000-0000-4000-8000-%012d", n)) }
func AppendBasisMaterials(ctx context.Context, tx pgx.Tx) error {
	var existing bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence WHERE id=$1 OR id=$2)`, ID(1100), ID(2100)).Scan(&existing); err != nil {
		return err
	}
	if existing {
		return fmt.Errorf("basis fixtures already exist; no overwrite")
	}
	for k, workspace := range []string{"11111111-1111-4111-8111-111111111111", "12121212-1212-4212-8212-121212121212"} {
		ws := uuid.MustParse(workspace)
		base := (k + 1) * 1000
		delivery, version := ID(base+2), ID(base+1)
		actor := uuid.MustParse("73000000-0000-4000-8000-000000000002")
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_operation WHERE id=$1 AND workspace_id=$2 AND dataset_version_id=$3 AND status='BLOCKED' AND provider_name='SYNTHETIC_NO_PROVIDER')`, delivery, ws, version).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("explicit synthetic blocked target required")
		}
		for i := 0; i < 30; i++ {
			kind := "RECEIPT_ATTESTATION"
			if i == 1 {
				kind = "QUALITY_RESULT"
			}
			metadata := map[string]any{"syntheticAcceptance": true, "verified": true, "contentSha256": strings.Repeat("b", 64), "receiver": "claimed-but-not-authoritative", "privateMarker": "PRIVATE_BASIS_METADATA_MUST_NOT_LEAK"}
			if i == 3 {
				metadata["oversized"] = strings.Repeat("X", 70000)
			}
			rec := evidence.Record{ID: ID(base + 100 + i), WorkspaceID: ws, EvidenceType: kind, Title: "PRIVATE_TITLE_MUST_NOT_LEAK", SourceType: "SYNTHETIC_ACCEPTANCE", SourceID: &delivery, StorageURI: "https://fixture.invalid/PRIVATE_URI_MUST_NOT_LEAK", Metadata: metadata, CreatedBy: &actor, CreatedAt: time.Date(2026, 10, 24, 0, 0, i, 0, time.UTC)}
			relations := []evidence.Relation{}
			if i != 4 {
				relations = append(relations, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: delivery, RelationType: "SUPPORTS"})
			}
			if i != 5 {
				relations = append(relations, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: version, RelationType: "SUPPORTS"})
			}
			if i == 2 {
				// New bad-hash record only. Never mutate an existing immutable Evidence.
				_, err := tx.Exec(ctx, `INSERT INTO evidence(id,workspace_id,evidence_type,title,source_type,source_id,storage_uri,hash_algorithm,hash_value,metadata,created_at,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,'SHA256-EVIDENCE-V2',repeat('0',64),'{}'::jsonb,$8,$9)`, rec.ID, ws, kind, rec.Title, rec.SourceType, delivery, rec.StorageURI, rec.CreatedAt, actor)
				if err != nil {
					return err
				}
				for _, rel := range relations {
					if _, err = tx.Exec(ctx, `INSERT INTO evidence_relation(evidence_id,object_type,object_id,relation_type)VALUES($1,$2,$3,$4)`, rec.ID, rel.ObjectType, rel.ObjectID, rel.RelationType); err != nil {
						return err
					}
				}
			} else {
				if _, err := evidence.Append(ctx, tx, rec, relations...); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
