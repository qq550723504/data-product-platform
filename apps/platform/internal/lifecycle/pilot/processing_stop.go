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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"io"
	"time"
)

type ProcessingOwnerIdentity struct {
	Run, Workspace, Execution                                             uuid.UUID
	HelperID, ImageSHA256, BinarySHA256, PinsSHA256, RuntimeClosureSHA256 string
}
type ProcessingOwnerStop struct {
	ProcessingOwnerIdentity
	Nonce                                                                    uuid.UUID
	ObservedAt                                                               time.Time
	Exited, RestartFenced, CredentialsDisabled, ConfigDisabled, ClosedCgroup bool
}
type ProcessingStopVerifier struct {
	Key      ed25519.PublicKey
	Expected ProcessingOwnerIdentity
	Proof    SignedClosedReferenceExecutor
}

func (v ProcessingStopVerifier) Validate(at time.Time) (ProcessingOwnerStop, error) {
	var p ProcessingOwnerStop
	if len(v.Key) != ed25519.PublicKeySize || len(v.Proof.Body) > 16384 || !ed25519.Verify(v.Key, v.Proof.Body, v.Proof.Signature) {
		return p, domain.ErrTerminationUnknown
	}
	d := json.NewDecoder(bytes.NewReader(v.Proof.Body))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&p) != nil || d.Decode(&extra) != io.EOF || p.ProcessingOwnerIdentity != v.Expected || p.Run == uuid.Nil || p.Workspace == uuid.Nil || p.Execution == uuid.Nil || p.Nonce == uuid.Nil || p.ObservedAt.IsZero() || at.Before(p.ObservedAt) || at.Sub(p.ObservedAt) > time.Minute || !p.Exited || !p.RestartFenced || !p.CredentialsDisabled || !p.ConfigDisabled || !p.ClosedCgroup {
		return p, domain.ErrTerminationUnknown
	}
	for _, s := range []string{p.HelperID, p.ImageSHA256, p.BinarySHA256, p.PinsSHA256, p.RuntimeClosureSHA256} {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 {
			return p, domain.ErrTerminationUnknown
		}
	}
	return p, nil
}
func (v ProcessingStopVerifier) VerifyTermination(ctx context.Context, tx pgx.Tx, pin domain.ReaderPin, outcome string, at time.Time) (domain.ReaderTermination, error) {
	p, e := v.Validate(at)
	if e != nil || tx == nil || pin.Kind != domain.FixtureProcessing || pin.WorkspaceID != p.Workspace || pin.ExecutionID == nil || *pin.ExecutionID != p.Execution || outcome != domain.ReaderCancelled {
		return domain.ReaderTermination{}, domain.ErrTerminationUnknown
	}
	var status string
	var pending bool
	if e = tx.QueryRow(ctx, `SELECT status,EXISTS(SELECT 1 FROM lifecycle_processing_output_intent WHERE execution_id=$1) FROM execution WHERE id=$1 AND workspace_id=$2`, p.Execution, p.Workspace).Scan(&status, &pending); e != nil {
		return domain.ReaderTermination{}, e
	}
	if status != "FAILED" || pending {
		return domain.ReaderTermination{}, domain.ErrTerminationUnknown
	}
	// Canonical full membership is captured from immutable pin rows, not caller
	// pin assertions. Normalize using the same Go structs on host and receiver.
	rows, e := tx.Query(ctx, `SELECT id FROM lifecycle_reader_pin WHERE workspace_id=$1 AND execution_id=$2 ORDER BY id`, p.Workspace, p.Execution)
	if e != nil {
		return domain.ReaderTermination{}, e
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return domain.ReaderTermination{}, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return domain.ReaderTermination{}, e
	}
	var pins []domain.ReaderPin
	repo := &infrastructure.PostgresRepository{}
	for _, id := range ids {
		x, e := repo.GetReaderPin(ctx, tx, p.Workspace, id)
		if e != nil {
			return domain.ReaderTermination{}, e
		}
		pins = append(pins, x)
	}
	b, e := json.Marshal(pins)
	if e != nil {
		return domain.ReaderTermination{}, e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != p.PinsSHA256 {
		return domain.ReaderTermination{}, domain.ErrTerminationUnknown
	}
	proofHash := sha256.Sum256(v.Proof.Body)
	id := uuid.New()
	_, e = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: p.Workspace, EvidenceType: "PROCESSING_OWNER_STOP_VERIFIED", Title: "Closed native owner stopped", SourceType: "LIFECYCLE_READER_TERMINATION", SourceID: &id, Metadata: map[string]any{"execution_id": p.Execution, "pin_id": pin.ID, "generation": pin.Generation, "helper_id": p.HelperID, "pins_sha256": p.PinsSHA256, "signed_body_sha256": hex.EncodeToString(proofHash[:]), "signed_body": v.Proof.Body, "signature": v.Proof.Signature, "public_key": []byte(v.Key), "nonce": p.Nonce, "observed_at": p.ObservedAt}}, evidence.Relation{ObjectType: "EXECUTION", ObjectID: p.Execution, RelationType: "OWNER_STOP"})
	return domain.ReaderTermination{ID: id, WorkspaceID: pin.WorkspaceID, PinID: pin.ID, ReaderID: pin.ReaderID, Generation: pin.Generation, Outcome: outcome, ObservedAt: p.ObservedAt}, e
}
