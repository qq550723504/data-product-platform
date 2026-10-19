package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// This is a finite acceptance composition, not a second workflow runtime.
// Recovery remains the existing Outbox -> Asynq path. A provider 204 or an
// UNKNOWN observation never satisfies this immutable Core receipt boundary.
func predecessorCopies(group, stage string) ([]uuid.UUID, error) {
	stages := map[string][]string{
		"business-races": {"P1", "P2", "T1", "T2"},
		"late-hold":      {"H2"},
		"mixed":          {"M1A", "M1B"},
	}[group]
	var copies []uuid.UUID
	for _, name := range stages {
		if name == stage {
			return copies, nil
		}
		cp, err := fixedTarget(group, name)
		if err != nil {
			return nil, err
		}
		copies = append(copies, cp.ID)
	}
	return nil, lc.ErrStorageScope
}

func requirePriorAbsences(ctx context.Context, q rowQuerier, scope pilot.RemainingScope, stage string) error {
	prior, err := predecessorCopies(scope.Group, stage)
	if err != nil {
		return err
	}
	if len(prior) == 0 {
		return nil
	}
	var confirmed int
	if err = q.QueryRow(ctx, `SELECT count(DISTINCT copy_id) FROM lifecycle_copy_absence_receipt
 WHERE workspace_id=$1 AND copy_id=ANY($2::uuid[])`, scope.Workspace, prior).Scan(&confirmed); err != nil {
		return fmt.Errorf("read predecessor absence receipts: %w", err)
	}
	if confirmed != len(prior) {
		return fmt.Errorf("%w: %s requires %d committed predecessor absence receipts, found %d", lc.ErrStorageDenied, stage, len(prior), confirmed)
	}
	return nil
}
