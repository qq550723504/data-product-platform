package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

func remainingCloseH1(ctx context.Context, r remainingConfig, pool *pgxpool.Pool) error {
	if r.Group != "late-hold" {
		return lc.ErrStorageScope
	}
	pub, e := hex.DecodeString(r.ClosurePublicKey)
	if e != nil {
		return lc.ErrStorageScope
	}
	var proof pilot.SignedClosedReferenceExecutor
	if e = readRemaining("/run/private/closed-H1.json", &proof); e != nil {
		return e
	}
	var closed pilot.RemainingClosedSender
	if json.Unmarshal(proof.Body, &closed) != nil {
		return lc.ErrStorageContract
	}
	verifier := pilot.RemainingNoSendVerifier{PublicKey: pub, Proof: proof, Execution: r.Batch, CandidateSHA256: r.ClosureCandidateSHA256, Group: r.Group, Owners: r.ClosureOwners}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, remainingAuthority{r}, nil, nil)
	service := lc.NewDispositionService(o, nil, nil, verifier)
	var h1 struct{ Attempt struct{ ID string } }
	// Resolve only the current execution's H1 attempt through immutable approval
	// and dispatcher facts. No caller supplies an arbitrary attempt or outcome.
	scope, _ := pilot.RemainingScopeFor(r.Group)
	if e = pool.QueryRow(ctx, `SELECT a.id::text FROM lifecycle_provider_attempt a JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id JOIN lifecycle_disposition_approval p ON p.id=d.approval_id WHERE a.plan_id=$1 AND a.copy_id=$2 AND p.basis_ref=$3 AND p.principal_ref=$4`, scope.Plans[1].ID, scope.Copies[1].ID, "REMAINING_NON_DELETE_REHEARSAL:"+r.Batch.String(), "remaining-fixture:"+r.Batch.String()).Scan(&h1.Attempt.ID); e != nil {
		return e
	}
	id, e := uuid.Parse(h1.Attempt.ID)
	if e != nil {
		return e
	}
	fact, e := service.RecordProviderObservation(ctx, lc.RecordProviderObservationCommand{CommandHeader: lc.CommandHeader{WorkspaceID: r.Base.WorkspaceID, IdempotencyKey: r.Batch.String() + ":H1-closed-NOT_SENT"}, AttemptID: id, ObservationRef: "remaining-closed:" + closed.Nonce.String()})
	if e != nil {
		return e
	}
	return write(r.Base, "remaining-H1-NOT_SENT.json", map[string]any{"observation": fact, "signed_closure": proof, "resend": 0, "absence": 0})
}
