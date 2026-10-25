package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"os"
	"path/filepath"
	"time"
)

func credentialClosure(ctx context.Context, c config, pool *pgxpool.Pool, cs []ld.Copy, mode string) error {
	if !c.LC13Continuation || c.RootAccess != "" || c.IssuerSecret != "" {
		return lc.ErrStorageScope
	}
	all, e := planeCopies(ctx, c, pool)
	if e != nil {
		return e
	}
	o, _, _, _, e := planeServices(ctx, c, pool, all)
	if e != nil {
		return e
	}
	if mode == "credential-restrict" {
		_, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "credential-forensic-stop-new-use"}, DatasetVersionID: cs[1].DatasetVersionID, BasisRef: "Verified closed native fixture owner; no future use"})
		return e
	}
	if mode == "credential-subjects" {
		path := filepath.Join(c.OutputDirectory, "credential-confirm.json")
		if c.RunID.String() == "f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3" {
			path = filepath.Join(c.OutputDirectory, "credential-forensic.json")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var facts struct {
			Facts []struct{ Operation, Call uuid.UUID }
		}
		if json.Unmarshal(b, &facts) != nil {
			return lc.ErrStorageScope
		}
		tx, e := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		subjects := []pilot.CredentialStopSubject{}
		for _, f := range facts.Facts {
			op, e := deliveryinfra.NewPostgresRepository(pool).GetOperation(ctx, tx, f.Operation, false)
			if e != nil {
				return e
			}
			subjects = append(subjects, pilot.CredentialStopSubject{Operation: op.ID, Copy: cs[1].ID, DenialCall: f.Call, ProviderRequestKey: op.ProviderRequestKey, CapabilityRef: op.CredentialRef, CapabilityHash: op.CredentialHash, Principal: op.PrincipalRef, Consumer: op.EffectiveConsumerRef, Purpose: op.Purpose, Action: op.Action, Scope: op.ScopeRef, Channel: op.DeliveryChannel, Mode: op.DeliveryMode, ExpiresAt: op.ProviderCredentialExpiresAt})
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		canonical, _ := json.Marshal(subjects)
		h := sha256.Sum256(canonical)
		return write(c, "credential-subjects.json", map[string]any{"Subjects": subjects, "SHA256": hex.EncodeToString(h[:])})
	}
	if mode != "credential-release" {
		return lc.ErrStorageScope
	}
	key, e := hex.DecodeString(c.CredentialStopPublicKey)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return lc.ErrStorageScope
	}
	var proof pilot.SignedClosedReferenceExecutor
	b, e := os.ReadFile("/run/private/credential-stop.json")
	if e != nil || json.Unmarshal(b, &proof) != nil {
		return lc.ErrStorageScope
	}
	owner := pilot.CredentialStopOwner{Key: key, Expected: c.CredentialStopIdentity, Proof: proof}
	p, e := owner.Validate(time.Now().UTC())
	if e != nil {
		return e
	}
	refs := lc.NewFixtureReferenceService(o, owner)
	facts := []map[string]any{}
	for _, subject := range p.Subjects {
		var requirement uuid.UUID
		if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_reference_requirement WHERE workspace_id=$1 AND copy_id=$2 AND delivery_operation_id=$3 AND kind='DELIVERY'`, c.WorkspaceID, subject.Copy, subject.Operation).Scan(&requirement); e != nil {
			return e
		}
		spec, _ := contentreference.Lookup("DELIVERY")
		cmd := lc.ReleaseFixtureReferenceCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "native-credential-closed:" + subject.Operation.String()}, RequirementID: requirement, CopyID: subject.Copy, Reference: contentreference.Reference{Kind: "DELIVERY", SubjectID: subject.Operation}, BasisRef: "Signed native disable/GET denial and closed local owner", AcknowledgedLoss: spec.Loss, OwnerVerifiedQuiescent: true}
		result, e := refs.ReleaseFixtureReference(ctx, cmd)
		if e != nil {
			return e
		}
		replay, e := refs.ReleaseFixtureReference(ctx, cmd)
		if e != nil || !replay.Replayed || result.ID != replay.ID {
			return errors.New("credential release replay mismatch")
		}
		facts = append(facts, map[string]any{"operation": subject.Operation, "requirement": requirement, "release": result.ID, "replay": replay.Replayed})
	}
	return write(c, "credential-release.json", map[string]any{"facts": facts, "signed_proof": proof, "public_key": c.CredentialStopPublicKey, "native_delete": 0, "historical_issued_preserved": true})
}
