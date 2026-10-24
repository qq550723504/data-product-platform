package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

const PostV28Execution = "4fd5a7e7-779e-41d7-a35a-3bab7dd615b5"
const PostV28Candidate = "bb0924c1814b0bcdecd33d3b57a21a24beaafce2a3d64e10e5dac9aba1947f63"
const PostV28Bindings = "8d021af81d905cc6de255bf84e460fc0b416d5af5bacc8388cfbb2b423a60624"

// Exact failed-generation identities read from the retained Core, not selectors.
func PostV28BusinessDispatches() []OldBusinessDispatch {
	d := PostV22BusinessDispatches()
	ids := [][3]string{
		{"eacd910a-3ecd-448e-9c87-13bdf14d8817", "4d930776-fd98-4a86-8ed2-0a1eaad7dd34", "b0d65097-6991-4e09-8c76-a02158a0b001"},
		{"002be431-180b-44c3-9f0b-747aaeaaa17a", "4d5b1b64-fd97-4626-877f-09195725f11b", "7e11f4d9-c422-4f5f-9bda-7053cfa45516"},
		{"caade58d-9243-4bd3-b4c8-147d104858ca", "fc9aca0e-dd87-40cc-8cbc-b629a8c9e2c4", "aa61c0e2-f86e-4857-aafa-ee81f92dd964"},
	}
	for i := range d {
		d[i].Dispatch = uuid.MustParse(ids[i][0])
		d[i].Attempt = uuid.MustParse(ids[i][1])
		d[i].Approval = uuid.MustParse(ids[i][2])
	}
	return d
}

// P2 actually sent. It is deliberately excluded from this NOT_SENT verifier.
type PostV28NoSendVerifier PostV22NoSendVerifier

func (v PostV28NoSendVerifier) Validate(at time.Time, ref string) (ClosedOldBusinessDispatches, error) {
	return validateClosedBusinessProof(PostV22NoSendVerifier(v), at, ref, PostV28Execution, PostV28Candidate, PostV28Bindings, PostV22Core, "post-v28-closed:", PostV28BusinessDispatches()[1:])
}

func (v PostV28NoSendVerifier) VerifyProviderObservation(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	if _, e := v.Validate(at, ref); e != nil {
		return domain.ProviderObservation{}, e
	}
	var index = -1
	targets := PostV28BusinessDispatches()
	for i := 1; i < len(targets); i++ {
		if targets[i].Attempt == a.ID {
			index = i
		}
	}
	if index < 0 || a.Validate() != nil || a.WorkspaceID.String() != "a4aa2689-6e8f-4180-a567-dfb875407eac" {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	d := targets[index]
	if a.PlanID != d.Plan || a.CopyID != d.Copy || a.DispatchID != d.Dispatch {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	history := []OldBusinessDispatch{PostV22BusinessDispatches()[index], PostV27BusinessDispatches()[index], d}
	executions := []string{PostV22Execution, PostV27Execution, PostV28Execution}
	for i, h := range history {
		var eligible bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d
 JOIN lifecycle_disposition_approval p ON p.id=d.approval_id
 JOIN lifecycle_approval_revocation r ON r.approval_id=p.id
 JOIN lifecycle_provider_attempt a ON a.dispatch_id=d.id
 WHERE d.id=$1 AND d.workspace_id=$2 AND d.plan_id=$3 AND d.copy_id=$4
 AND p.id=$5 AND p.plan_hash=$6 AND p.basis_ref=$7 AND p.principal_ref=$8
 AND a.id=$9 AND a.workspace_id=$2 AND a.plan_id=$3 AND a.copy_id=$4)`, h.Dispatch, a.WorkspaceID, h.Plan, h.Copy, h.Approval, h.PlanHash, "REMAINING_NON_DELETE_REHEARSAL:"+executions[i], "remaining-fixture:"+executions[i], h.Attempt).Scan(&eligible)
		if e != nil {
			return domain.ProviderObservation{}, e
		}
		if !eligible {
			return domain.ProviderObservation{}, lc.ErrStorageDenied
		}
		if i < 2 {
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_observation o
 JOIN lifecycle_dispatch_release r ON r.observation_id=o.id AND r.workspace_id=o.workspace_id
 WHERE o.attempt_id=$1 AND o.workspace_id=$2 AND r.dispatch_id=$3 AND o.certainty='NOT_SENT' AND o.outcome='NOT_SENT' AND o.invoked_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$1 AND (certainty<>'NOT_SENT' OR outcome<>'NOT_SENT' OR invoked_at IS NOT NULL))
 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$1)`, h.Attempt, a.WorkspaceID, h.Dispatch).Scan(&eligible)
			if e != nil {
				return domain.ProviderObservation{}, e
			}
			if !eligible {
				return domain.ProviderObservation{}, lc.ErrStorageDenied
			}
		}
	}
	var conflict bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$1)
 OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$1)
 OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$2)
 OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$3)
 OR EXISTS(SELECT 1 FROM lifecycle_provider_attempt WHERE copy_id=$3 AND id NOT IN($1,$4,$5))
 OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch WHERE copy_id=$3 AND id NOT IN($2,$6,$7))`, a.ID, d.Dispatch, d.Copy, history[0].Attempt, history[1].Attempt, history[0].Dispatch, history[1].Dispatch).Scan(&conflict)
	if e != nil {
		return domain.ProviderObservation{}, e
	}
	if conflict {
		return domain.ProviderObservation{}, lc.ErrStorageDenied
	}
	sum := sha256.Sum256(v.Proof.Body)
	return domain.ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID, Certainty: domain.InvocationNotSent, Outcome: domain.InvocationNotSent, ProofSHA256: hex.EncodeToString(sum[:]), ObservedAt: at}, nil
}
