package pilot

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Host attestation is captured by trusted composition BEFORE the transaction.
// This verifier performs signature/current DB checks under the existing fence,
// plan, dispatch and attempt locks; it performs no external I/O under those locks.
type ClosedReferenceExecutor struct {
	Nonce                                                                        uuid.UUID
	CheckedAt                                                                    time.Time
	Environment, PlanHash, HelperID, BinarySHA256, ScriptSHA256, CandidateSHA256 string
}
type SignedClosedReferenceExecutor struct {
	// Preserve the exact signed bytes across JSON indentation/serialization.
	// JSON encodes []byte as base64; RawMessage would be reformatted by MarshalIndent.
	Body      []byte
	Signature []byte
}
type ReferenceNoSendVerifier struct {
	Key   ed25519.PublicKey
	Proof SignedClosedReferenceExecutor
}

func (v ReferenceNoSendVerifier) Validate(at time.Time, ref string) (ClosedReferenceExecutor, error) {
	var p ClosedReferenceExecutor
	if len(v.Key) != ed25519.PublicKeySize || len(v.Proof.Body) > 16384 || !ed25519.Verify(v.Key, v.Proof.Body, v.Proof.Signature) || json.Unmarshal(v.Proof.Body, &p) != nil {
		return p, lc.ErrStorageContract
	}
	digest, e := hex.DecodeString(p.CandidateSHA256)
	if e != nil || len(digest) != 32 || p.Nonce == uuid.Nil || ref != "closed-executor:"+p.Nonce.String() || p.Environment != ReferenceEnvironment || p.PlanHash != ReferencePlanHash || p.HelperID != "fb92bdcd150127c50163448de9d918f3a2d9a18e2384274b30febfaf647bdb87" || p.BinarySHA256 != "5e04e88bbd65880a5e65da5795dcdc116021512e04d27a200ec0d03f83e972c8" || p.ScriptSHA256 != "24e1d6d19394208016ee1d9dd0ddaa06669880d2cebae03b4c7996db0303fd1c" || p.CheckedAt.IsZero() || at.Before(p.CheckedAt) || at.Sub(p.CheckedAt) > time.Minute {
		return p, lc.ErrStorageContract
	}
	return p, nil
}
func ReferenceTrialAttempts() map[uuid.UUID]struct{ Copy, Dispatch uuid.UUID } {
	return map[uuid.UUID]struct{ Copy, Dispatch uuid.UUID }{
		uuid.MustParse("0a86461d-d5c2-4c00-99ae-0d44ac6dde2a"): {uuid.MustParse("8b1ff30c-3695-4847-b4cf-cc92cd6a399c"), uuid.MustParse("8b9b4d0c-df37-41b5-b3d1-0630deae8675")},
		uuid.MustParse("d8a52b7b-9cad-4ab4-a8d5-286d37ceafb9"): {uuid.MustParse("0a239dc6-9e1d-49eb-8604-e22a93e7b6a0"), uuid.MustParse("aa0dc82a-06b4-4b0d-a066-33d19ccc8f45")},
	}
}
func (v ReferenceNoSendVerifier) VerifyProviderObservation(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	if _, e := v.Validate(at, ref); e != nil {
		return domain.ProviderObservation{}, e
	}
	expected, ok := ReferenceTrialAttempts()[a.ID]
	if !ok || a.Validate() != nil || a.WorkspaceID != ReferenceWorkspace || a.PlanID != ReferencePlan || a.CopyID != expected.Copy || a.DispatchID != expected.Dispatch {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	var valid, conflicting bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d JOIN lifecycle_disposition_approval x ON x.id=d.approval_id WHERE d.id=$1 AND d.workspace_id=$2 AND d.plan_id=$3 AND d.copy_id=$4 AND x.id='34274662-4973-4621-ab4d-b89c0493f1dc' AND x.plan_hash=$5 AND x.basis_ref='CONTROLLED_NON_DELETE_ADMISSION_EXERCISE'), EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$6) OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$6) OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1) OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE dispatch_id=$1)`, a.DispatchID, a.WorkspaceID, a.PlanID, a.CopyID, ReferencePlanHash, a.ID).Scan(&valid, &conflicting)
	if e != nil {
		return domain.ProviderObservation{}, e
	}
	if !valid || conflicting {
		return domain.ProviderObservation{}, lc.ErrStorageDenied
	}
	hash := sha256.Sum256(v.Proof.Body)
	return domain.ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID, Certainty: domain.InvocationNotSent, Outcome: domain.InvocationNotSent, ProofSHA256: hex.EncodeToString(hash[:]), ObservedAt: at}, nil
}
