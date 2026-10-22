package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type TerminateProcessingReadersCommand struct {
	CommandHeader
	ExecutionID uuid.UUID
	Stops       []TerminateFixtureReaderCommand
}

// This batch is one business transaction, not nested Manager.Do calls. Every
// original input pin must be included; proof failure rolls back all terminations.
func (s *OccupancyService) TerminateProcessingReaders(ctx context.Context, c TerminateProcessingReadersCommand) (FactResult, error) {
	if s == nil || s.termination == nil || c.ExecutionID == uuid.Nil || len(c.Stops) == 0 || len(c.Stops) > 16 {
		return FactResult{}, domain.ErrTerminationUnknown
	}
	return s.command(ctx, c.CommandHeader, "LIFECYCLE.TERMINATE_PROCESSING_READERS", CapabilityTerminateReader, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		var expected int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reader_pin WHERE workspace_id=$1 AND execution_id=$2`, c.WorkspaceID, c.ExecutionID).Scan(&expected); e != nil {
			return uuid.Nil, e
		}
		if expected != len(c.Stops) {
			return uuid.Nil, domain.ErrTerminationUnknown
		}
		seen := map[uuid.UUID]bool{}
		copies := []uuid.UUID{}
		for _, stop := range c.Stops {
			if stop.WorkspaceID != c.WorkspaceID || stop.PinID == uuid.Nil || seen[stop.PinID] || stop.Outcome != domain.ReaderCancelled {
				return uuid.Nil, domain.ErrTerminationUnknown
			}
			seen[stop.PinID] = true
			p, e := s.repo.GetReaderPin(ctx, tx, c.WorkspaceID, stop.PinID)
			if e != nil {
				return uuid.Nil, e
			}
			if p.Kind != domain.FixtureProcessing || p.ExecutionID == nil || *p.ExecutionID != c.ExecutionID || p.Generation != stop.Generation {
				return uuid.Nil, domain.ErrTerminationUnknown
			}
			copies = append(copies, p.CopyID)
		}
		if _, e := s.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, copies); e != nil {
			return uuid.Nil, e
		}
		var ids []uuid.UUID
		for _, stop := range c.Stops {
			id, e := s.terminateReaderTx(ctx, tx, caller, stop)
			if e != nil {
				return uuid.Nil, e
			}
			ids = append(ids, id)
		}
		_, at, e := s.authorize(ctx, tx, c.WorkspaceID, CapabilityTerminateReader, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		return c.ExecutionID, appendOccupancyFacts(ctx, tx, "LifecycleProcessingOwnerStopped", "EXECUTION", c.ExecutionID, caller, rev, at, map[string]any{"termination_ids": ids, "pin_count": len(ids), "outcome": domain.ReaderCancelled})
	})
}
