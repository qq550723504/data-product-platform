package pilot

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

type RemainingClosedSender struct {
	Execution, Nonce                                                                 uuid.UUID
	CandidateSHA256, Group                                                           string
	At                                                                               time.Time
	Owners                                                                           map[string]string // exact stopped container IDs -> frozen binary hashes
	StoppedPIDZero, NoRestart, ConfigDisabled, DeleteIdentityDisabled, GatewaySealed bool
}
type RemainingNoSendVerifier struct {
	PublicKey              ed25519.PublicKey
	Proof                  SignedClosedReferenceExecutor
	Execution              uuid.UUID
	CandidateSHA256, Group string
	Owners                 map[string]string
}

func (v RemainingNoSendVerifier) VerifyProviderObservation(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	var proof RemainingClosedSender
	if len(v.PublicKey) != ed25519.PublicKeySize || len(v.Proof.Body) > 16384 || !ed25519.Verify(v.PublicKey, v.Proof.Body, v.Proof.Signature) || json.Unmarshal(v.Proof.Body, &proof) != nil || proof.Execution != v.Execution || proof.Nonce == uuid.Nil || ref != "remaining-closed:"+proof.Nonce.String() || proof.CandidateSHA256 != v.CandidateSHA256 || len(proof.CandidateSHA256) != 64 || proof.Group != "late-hold" || proof.Group != v.Group || len(v.Owners) == 0 || !reflect.DeepEqual(v.Owners, proof.Owners) || !proof.StoppedPIDZero || !proof.NoRestart || !proof.ConfigDisabled || !proof.DeleteIdentityDisabled || !proof.GatewaySealed || at.Before(proof.At) || at.Sub(proof.At) > time.Minute {
		return domain.ProviderObservation{}, lc.ErrStorageContract
	}
	for id, binary := range proof.Owners {
		if raw, e := hex.DecodeString(id); e != nil || len(raw) != 32 {
			return domain.ProviderObservation{}, lc.ErrStorageContract
		}
		if raw, e := hex.DecodeString(binary); e != nil || len(raw) != 32 {
			return domain.ProviderObservation{}, lc.ErrStorageContract
		}
	}
	scope, _ := RemainingScopeFor("late-hold")
	if a.Validate() != nil || a.WorkspaceID != scope.Workspace || a.PlanID != scope.Plans[1].ID || a.CopyID != scope.Copies[1].ID {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	var eligible, conflict bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d JOIN lifecycle_disposition_approval p ON p.id=d.approval_id JOIN lifecycle_approval_revocation r ON r.approval_id=p.id WHERE d.id=$1 AND d.workspace_id=$2 AND d.plan_id=$3 AND d.copy_id=$4 AND p.basis_ref=$5 AND p.principal_ref=$6),EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$7) OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$7) OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1) OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE dispatch_id=$1)`, a.DispatchID, a.WorkspaceID, a.PlanID, a.CopyID, "REMAINING_NON_DELETE_REHEARSAL:"+v.Execution.String(), "remaining-fixture:"+v.Execution.String(), a.ID).Scan(&eligible, &conflict)
	if err != nil {
		return domain.ProviderObservation{}, err
	}
	if !eligible || conflict {
		return domain.ProviderObservation{}, lc.ErrStorageDenied
	}
	sum := sha256.Sum256(v.Proof.Body)
	return domain.ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID, Certainty: domain.InvocationNotSent, Outcome: domain.InvocationNotSent, ProofSHA256: hex.EncodeToString(sum[:]), ObservedAt: at}, nil
}
