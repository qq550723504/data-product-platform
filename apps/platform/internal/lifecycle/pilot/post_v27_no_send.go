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

// These are failed execution facts, never a new sender/candidate selector.
const PostV27Execution = "6f40bca9-cfe2-4362-ad2b-4e38bbbd8453"
const PostV27Candidate = "20d08f0a2d334e779b64f0c082515131c285d9fd005c6fbaa94294b147f38d42"
const PostV27Bindings = "77ce0be4d5dccd95a276a5b4c3f060689bea870f4f3caef1a827ef81154efa24"

func PostV27BusinessDispatches() []OldBusinessDispatch {
	prior := PostV22BusinessDispatches()
	ids := [][3]string{
		{"5e06d692-648a-4f8c-a13e-8bf1b1452c7b", "ba5bec6b-84ec-4418-a63f-4b08601ccb76", "da615e44-5c9a-4c46-8614-a0a0012c444b"},
		{"e6067cd3-0fdf-4952-b18d-12f9913534df", "c1f32764-d3d3-4752-ba48-fc88373c0210", "ffef8470-5b80-48fb-ae7b-821e17f36266"},
		{"b7f4e168-f8e8-42d4-bea7-1da6a8ec0de5", "3c731704-cc91-4717-95fc-828ced05b46c", "1cabd635-f9ef-4b64-93c0-24cae172fccc"},
	}
	for i := range prior {
		prior[i].Dispatch = uuid.MustParse(ids[i][0])
		prior[i].Attempt = uuid.MustParse(ids[i][1])
		// Pinned by the fresh read-only Core audit; never caller-selected/latest.
		prior[i].Approval = uuid.MustParse(ids[i][2])
	}
	return prior
}

type PostV27NoSendVerifier PostV22NoSendVerifier

func (v PostV27NoSendVerifier) Validate(at time.Time, ref string) (ClosedOldBusinessDispatches, error) {
	for _, target := range PostV27BusinessDispatches() {
		if target.Approval == uuid.Nil {
			return ClosedOldBusinessDispatches{}, lc.ErrStorageContract
		}
	}
	return validateClosedBusinessProof(PostV22NoSendVerifier(v), at, ref, PostV27Execution, PostV27Candidate, PostV27Bindings, PostV22Core, "post-v27-closed:", PostV27BusinessDispatches())
}

func (v PostV27NoSendVerifier) VerifyProviderObservation(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	if _, err := v.Validate(at, ref); err != nil {
		return domain.ProviderObservation{}, err
	}
	var expected, historical *OldBusinessDispatch
	old := PostV22BusinessDispatches()
	for i, target := range PostV27BusinessDispatches() {
		if target.Attempt == a.ID {
			copy, previous := target, old[i]
			expected, historical = &copy, &previous
		}
	}
	if expected == nil || a.Validate() != nil || a.WorkspaceID.String() != "a4aa2689-6e8f-4180-a567-dfb875407eac" || a.PlanID != expected.Plan || a.CopyID != expected.Copy || a.DispatchID != expected.Dispatch {
		return domain.ProviderObservation{}, lc.ErrStorageScope
	}
	// Called inside the existing Command's workspace/plan/dispatch/attempt fence.
	// Only the one exact v22 predecessor may be ignored, and only after its
	// immutable NOT_SENT + release with no claim. UNKNOWN is never admissible.
	var eligible, conflict bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d
	 JOIN lifecycle_disposition_approval p ON p.id=d.approval_id JOIN lifecycle_approval_revocation r ON r.approval_id=p.id
	 WHERE d.id=$1 AND d.workspace_id=$2 AND d.plan_id=$3 AND d.copy_id=$4 AND p.id=$5 AND p.plan_hash=$6
	 AND p.basis_ref=$7 AND p.principal_ref=$8
	 AND EXISTS(SELECT 1 FROM lifecycle_provider_attempt WHERE id=$10 AND dispatch_id=$11 AND copy_id=$4 AND workspace_id=$2 AND plan_id=$3)),
	 EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$9)
	 OR EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$9)
	 OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1)
	 OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$4)
	 OR EXISTS(SELECT 1 FROM lifecycle_provider_attempt x WHERE x.copy_id=$4 AND x.id<>$9 AND NOT (
	 x.id=$10 AND x.dispatch_id=$11 AND x.workspace_id=$2 AND x.plan_id=$3
	 AND EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch od JOIN lifecycle_disposition_approval op ON op.id=od.approval_id
	 JOIN lifecycle_approval_revocation rv ON rv.approval_id=op.id
	 WHERE od.id=$11 AND od.copy_id=$4 AND od.workspace_id=$2 AND od.plan_id=$3 AND op.id=$12 AND op.plan_hash=$6
	 AND op.basis_ref=$13 AND op.principal_ref=$14)
	 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_send_claim WHERE attempt_id=$10)
	 AND EXISTS(SELECT 1 FROM lifecycle_provider_observation o
	 JOIN lifecycle_provider_attempt oa ON oa.id=o.attempt_id AND oa.workspace_id=o.workspace_id
	 JOIN lifecycle_dispatch_release rel ON rel.observation_id=o.id AND rel.workspace_id=o.workspace_id
	 WHERE o.attempt_id=$10 AND oa.dispatch_id=$11 AND oa.copy_id=$4 AND o.workspace_id=$2
	 AND o.certainty='NOT_SENT' AND o.outcome='NOT_SENT' AND o.invoked_at IS NULL AND rel.dispatch_id=$11)
	 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$10 AND (certainty<>'NOT_SENT' OR outcome<>'NOT_SENT' OR invoked_at IS NOT NULL))))
	 OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch WHERE copy_id=$4 AND id NOT IN($1,$11))`,
		a.DispatchID, a.WorkspaceID, a.PlanID, a.CopyID, expected.Approval, expected.PlanHash,
		"REMAINING_NON_DELETE_REHEARSAL:"+PostV27Execution, "remaining-fixture:"+PostV27Execution, a.ID,
		historical.Attempt, historical.Dispatch, historical.Approval,
		"REMAINING_NON_DELETE_REHEARSAL:"+PostV22Execution, "remaining-fixture:"+PostV22Execution).Scan(&eligible, &conflict)
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
