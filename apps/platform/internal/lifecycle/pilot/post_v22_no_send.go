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

const PostV22Execution = "8e41cfba-a41e-44dd-9ce8-d6365b3bb90e"
const PostV22Candidate = "f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243"
const PostV22Bindings = "8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5"
const PostV22Core = "9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0"

type OldBusinessDispatch struct {
	Case                                    string
	Plan, Copy, Dispatch, Attempt, Approval uuid.UUID
	PlanHash                                string
}

// Fixed historical identities, independently checked against the actual Core
// by audit 23587686-0a95-4eaa-a010-a8c896c9735b. This is not a target selector.
func PostV22BusinessDispatches() []OldBusinessDispatch {
	return []OldBusinessDispatch{
		{"P2", uuid.MustParse("bce1b1b1-d6e2-44fd-8b87-171142350ab9"), uuid.MustParse("6fabf3b5-76b8-4245-8f5d-01e64f610146"), uuid.MustParse("670f5351-103f-472f-aeea-1f29213eda7c"), uuid.MustParse("73d9251e-a128-47b1-8f5a-eaa35603c233"), uuid.MustParse("77b4d18c-8f1d-44e4-8844-696ba44a9a0f"), "d4758d9b64003d92aca60290fee1d2f22e7a857d2fe2be513d4a1ea2755ce6a3"},
		{"T1", uuid.MustParse("247a2ca9-d2f3-4384-bcee-b10add497175"), uuid.MustParse("f48755f1-976b-4f47-98da-70ead936a4c3"), uuid.MustParse("134315c3-bad8-4fda-a927-3ef71e2cca5e"), uuid.MustParse("bb6ecab6-53f1-4a9c-a333-f0cf6f4edd4a"), uuid.MustParse("16df6254-abd2-419d-bf6d-99a9d1d675d0"), "7c4daf909a375bc21fcf8dd09b14d99960ae1f919ce138eb517ac0593faf1c1c"},
		{"T2", uuid.MustParse("531c7b96-173f-4028-be97-2de9ed6c72f2"), uuid.MustParse("e09fd433-54cf-4e82-b8c4-098472505fa2"), uuid.MustParse("b9ebd875-8942-4303-8e17-41612dd8100c"), uuid.MustParse("933462ae-04d6-4da4-97de-067b62863e0a"), uuid.MustParse("4a20d7d4-b0a8-4dde-9b20-f133f6355dda"), "84fbafb913dba44e5ceb43d670203e55f0244fad62a2dc2fd9ebb10b87e0cab5"},
	}
}

type ClosedOldBusinessDispatches struct {
	AuditID, Nonce                                                                   uuid.UUID
	At                                                                               time.Time
	Execution, CandidateSHA256, BindingsSHA256, Core, EvidenceSHA256                 string
	Targets                                                                          []OldBusinessDispatch
	Owners                                                                           map[string]string
	StoppedPIDZero, NoRestart, ConfigDisabled, DeleteIdentityDisabled, GatewaySealed bool
}

// Server composition pins Owners and EvidenceSHA256 from its reviewed host
// manifest, never from a command caller. It mounts only the public key/proof.
type PostV22NoSendVerifier struct {
	PublicKey      ed25519.PublicKey
	Proof          SignedClosedReferenceExecutor
	AuditID        uuid.UUID
	Owners         map[string]string
	EvidenceSHA256 string
}

func (v PostV22NoSendVerifier) Validate(at time.Time, ref string) (ClosedOldBusinessDispatches, error) {
	var p ClosedOldBusinessDispatches
	if len(v.PublicKey) != ed25519.PublicKeySize || len(v.Proof.Body) > 16384 || !ed25519.Verify(v.PublicKey, v.Proof.Body, v.Proof.Signature) || json.Unmarshal(v.Proof.Body, &p) != nil ||
		v.AuditID == uuid.Nil || v.AuditID.String() == PostV22Execution || p.AuditID != v.AuditID || p.Nonce == uuid.Nil || ref != "post-v22-closed:"+p.Nonce.String() ||
		p.Execution != PostV22Execution || p.CandidateSHA256 != PostV22Candidate || p.BindingsSHA256 != PostV22Bindings || p.Core != PostV22Core ||
		!reflect.DeepEqual(p.Targets, PostV22BusinessDispatches()) || len(v.Owners) == 0 || !reflect.DeepEqual(p.Owners, v.Owners) || p.EvidenceSHA256 != v.EvidenceSHA256 ||
		!p.StoppedPIDZero || !p.NoRestart || !p.ConfigDisabled || !p.DeleteIdentityDisabled || !p.GatewaySealed || p.At.IsZero() || at.Before(p.At) || at.Sub(p.At) > time.Minute {
		return p, lc.ErrStorageContract
	}
	for _, s := range []string{v.EvidenceSHA256} {
		if b, e := hex.DecodeString(s); e != nil || len(b) != 32 {
			return p, lc.ErrStorageContract
		}
	}
	for id, binary := range v.Owners {
		for _, s := range []string{id, binary} {
			if b, e := hex.DecodeString(s); e != nil || len(b) != 32 {
				return p, lc.ErrStorageContract
			}
		}
	}
	return p, nil
}

func (v PostV22NoSendVerifier) VerifyProviderObservation(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	if _, err := v.Validate(at, ref); err != nil {
		return domain.ProviderObservation{}, err
	}
	var expected *OldBusinessDispatch
	for _, d := range PostV22BusinessDispatches() {
		if d.Attempt == a.ID {
			copy := d
			expected = &copy
		}
	}
	if expected == nil || a.Validate() != nil || a.WorkspaceID.String() != "a4aa2689-6e8f-4180-a567-dfb875407eac" || a.PlanID != expected.Plan || a.CopyID != expected.Copy || a.DispatchID != expected.Dispatch {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	// The existing Command holds the workspace fence, plan, dispatch and attempt
	// locks. No network I/O here; any claim/UNKNOWN/new attempt is a hard conflict.
	var eligible, conflict bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d
	 JOIN lifecycle_disposition_approval p ON p.id=d.approval_id JOIN lifecycle_approval_revocation r ON r.approval_id=p.id
	 WHERE d.id=$1 AND d.workspace_id=$2 AND d.plan_id=$3 AND d.copy_id=$4 AND p.id=$5 AND p.plan_hash=$6
	 AND p.basis_ref=$7 AND p.principal_ref=$8),
	 EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$9)
	 OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$9)
	 OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1)
	 OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$4)
	 OR EXISTS(SELECT 1 FROM lifecycle_provider_attempt WHERE copy_id=$4 AND id<>$9)
	 OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch WHERE copy_id=$4 AND id<>$1)`,
		a.DispatchID, a.WorkspaceID, a.PlanID, a.CopyID, expected.Approval, expected.PlanHash,
		"REMAINING_NON_DELETE_REHEARSAL:"+PostV22Execution, "remaining-fixture:"+PostV22Execution, a.ID).Scan(&eligible, &conflict)
	if err != nil {
		return domain.ProviderObservation{}, err
	}
	if !eligible || conflict {
		return domain.ProviderObservation{}, lc.ErrStorageDenied
	}
	sum := sha256.Sum256(v.Proof.Body)
	return domain.ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID,
		Certainty: domain.InvocationNotSent, Outcome: domain.InvocationNotSent, ProofSHA256: hex.EncodeToString(sum[:]), ObservedAt: at}, nil
}
