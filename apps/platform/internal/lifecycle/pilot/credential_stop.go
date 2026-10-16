package pilot

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"io"
	"time"
)

type CredentialOwnerIdentity struct {
	Run, Workspace                       uuid.UUID
	RuntimeClosureSHA256, SubjectsSHA256 string
}
type CredentialStopSubject struct {
	Operation, Copy, DenialCall                                                                                   uuid.UUID
	ProviderRequestKey, CapabilityRef, CapabilityHash, Principal, Consumer, Purpose, Action, Scope, Channel, Mode string
	ExpiresAt                                                                                                     *time.Time
}
type CredentialOwnerStop struct {
	CredentialOwnerIdentity
	Nonce                                                     uuid.UUID
	ObservedAt                                                time.Time
	OwnerExited, ConfigDisabled, ParentDisabled, ClosedCgroup bool
	Subjects                                                  []CredentialStopSubject
}
type CredentialStopOwner struct {
	Key      ed25519.PublicKey
	Expected CredentialOwnerIdentity
	Proof    SignedClosedReferenceExecutor
}

func (v CredentialStopOwner) Module() string { return "delivery" }
func (v CredentialStopOwner) Validate(at time.Time) (CredentialOwnerStop, error) {
	var p CredentialOwnerStop
	if len(v.Key) != ed25519.PublicKeySize || len(v.Proof.Body) > 32768 || !ed25519.Verify(v.Key, v.Proof.Body, v.Proof.Signature) {
		return p, contentreference.ErrReference
	}
	d := json.NewDecoder(bytes.NewReader(v.Proof.Body))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&p) != nil || d.Decode(&extra) != io.EOF || p.CredentialOwnerIdentity != v.Expected || p.Run == uuid.Nil || p.Workspace == uuid.Nil || p.Nonce == uuid.Nil || p.ObservedAt.IsZero() || at.Before(p.ObservedAt) || at.Sub(p.ObservedAt) > time.Minute || !p.OwnerExited || !p.ConfigDisabled || !p.ParentDisabled || !p.ClosedCgroup || len(p.Subjects) == 0 || len(p.Subjects) > 16 {
		return p, contentreference.ErrReference
	}
	for _, h := range []string{p.RuntimeClosureSHA256, p.SubjectsSHA256} {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 32 {
			return p, contentreference.ErrReference
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, s := range p.Subjects {
		if s.Operation == uuid.Nil || s.Copy == uuid.Nil || s.DenialCall == uuid.Nil || seen[s.Operation] {
			return p, contentreference.ErrReference
		}
		seen[s.Operation] = true
	}
	// Exact signed bytes bind the complete server-owned subject set. JSON
	// canonicalization is produced by this same Go struct before host signing.
	b, e := json.Marshal(p.Subjects)
	h := sha256.Sum256(b)
	if e != nil || hex.EncodeToString(h[:]) != p.SubjectsSHA256 {
		return p, contentreference.ErrReference
	}
	return p, nil
}
func (v CredentialStopOwner) Check(ctx context.Context, tx pgx.Tx, w, version uuid.UUID, ref contentreference.Reference) (contentreference.Spec, error) {
	spec, ok := contentreference.Lookup(ref.Kind)
	if !ok || ref.Kind != "DELIVERY" || ref.Validate() != nil || tx == nil {
		return spec, contentreference.ErrReference
	}
	p, e := v.Validate(time.Now().UTC())
	if e != nil || w != p.Workspace {
		return spec, contentreference.ErrReference
	}
	var subject *CredentialStopSubject
	for i := range p.Subjects {
		if p.Subjects[i].Operation == ref.SubjectID {
			subject = &p.Subjects[i]
		}
	}
	if subject == nil {
		return spec, contentreference.ErrReference
	}
	s := *subject
	op, e := deliveryinfra.NewPostgresRepository(nil).GetOperation(ctx, tx, s.Operation, false)
	if e != nil {
		return spec, e
	}
	if op.WorkspaceID != w || op.DatasetVersionID != version || op.ProviderName != "LC13_MINIO_REDEMPTION" || op.ProviderRequestKey != s.ProviderRequestKey || op.CredentialRef != s.CapabilityRef || op.CredentialHash != s.CapabilityHash || op.PrincipalRef != s.Principal || op.EffectiveConsumerRef != s.Consumer || op.Purpose != s.Purpose || op.Action != s.Action || op.ScopeRef != s.Scope || op.DeliveryChannel != s.Channel || op.DeliveryMode != s.Mode {
		return spec, contentreference.ErrReference
	}
	if (op.ProviderCredentialExpiresAt == nil) != (s.ExpiresAt == nil) || s.ExpiresAt != nil && !op.ProviderCredentialExpiresAt.Equal(*s.ExpiresAt) {
		return spec, contentreference.ErrReference
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id JOIN lifecycle_fixture_copy cp ON cp.id=c.copy_id WHERE c.id=$1 AND c.workspace_id=$2 AND c.copy_id=$3 AND cp.dataset_version_id=$4 AND c.operation='READ_CHECKPOINT_EXACT' AND x.http_status=403 AND x.invoked_at IS NOT NULL AND x.observed_at<=$5) AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_pin pin JOIN lifecycle_fixture_copy cp ON cp.id=pin.copy_id WHERE cp.dataset_version_id=$4 AND NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=pin.id))`, s.DenialCall, w, s.Copy, version, p.ObservedAt).Scan(&valid)
	if e != nil || !valid {
		return spec, contentreference.ErrReference
	}
	if op.Status == dd.StatusIssued {
		var decision string
		e = tx.QueryRow(ctx, `SELECT decision FROM delivery_credential_replay_decision WHERE delivery_operation_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, op.ID).Scan(&decision)
		if e != nil || decision != "BLOCKED" {
			return spec, contentreference.ErrReference
		}
	} else if op.Status != dd.StatusBlocked && op.Status != dd.StatusFailed {
		return spec, contentreference.ErrReference
	}
	hash := sha256.Sum256(v.Proof.Body)
	_, e = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: w, EvidenceType: "NATIVE_CREDENTIAL_OWNER_STOP_VERIFIED", Title: "Closed native credential owner contained", SourceType: "DELIVERY_OPERATION", SourceID: &op.ID, Metadata: map[string]any{"signed_body_sha256": hex.EncodeToString(hash[:]), "signed_body": v.Proof.Body, "signature": v.Proof.Signature, "public_key": []byte(v.Key), "runtime_closure_sha256": p.RuntimeClosureSHA256, "denial_call": s.DenialCall, "nonce": p.Nonce}}, evidence.Relation{ObjectType: "DELIVERY_OPERATION", ObjectID: op.ID, RelationType: "NATIVE_OWNER_STOP"})
	return spec, e
}
