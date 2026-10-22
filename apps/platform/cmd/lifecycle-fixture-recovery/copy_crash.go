package main

import (
	"context"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"os"
	"time"
)

type copyCrashPort struct {
	lc.CopyRecoveryPort
	scenario, barrier string
}

func (p copyCrashPort) DiscoverCopyTarget(ctx context.Context, op domain.CopyOperation) (lc.CopyDiscoveryProof, error) {
	if p.scenario == "copy-crash-before-lookup" {
		if write(p.barrier, map[string]any{"fault": "PROCESS_EXIT_AFTER_RUN_START_BEFORE_NATIVE_LOOKUP", "operation": op.ID}) == nil {
			os.Exit(91)
		}
	}
	proof, e := p.CopyRecoveryPort.DiscoverCopyTarget(ctx, op)
	if e != nil {
		return proof, e
	}
	if p.scenario == "copy-crash-after-lookup" {
		if write(p.barrier, map[string]any{"fault": "PROCESS_EXIT_AFTER_DURABLE_COPY_LOOKUP_BEFORE_BIND", "inventory_call_id": proof.InventoryCallID, "stat_call_id": proof.StatCallID, "exact_target": proof.Copy}) == nil {
			os.Exit(92)
		}
	}
	if p.scenario == "copy-hold-owner" || p.scenario == "copy-revoke-after-lookup" {
		if e = write(p.barrier, map[string]any{"fault": "OWNER_HELD_AFTER_DURABLE_LOOKUP_BEFORE_BIND", "inventory_call_id": proof.InventoryCallID, "stat_call_id": proof.StatCallID, "exact_target": proof.Copy}); e != nil {
			return proof, e
		}
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, e = os.Stat(p.barrier + ".release"); e == nil {
				break
			}
			select {
			case <-ctx.Done():
				return proof, ctx.Err()
			case <-ticker.C:
			}
		}
	}
	return proof, nil
}
func (p copyCrashPort) VerifyCopyBytes(ctx context.Context, op domain.CopyOperation, copy domain.Copy) (lc.CopyBytesProof, error) {
	proof, e := p.CopyRecoveryPort.VerifyCopyBytes(ctx, op, copy)
	if e != nil {
		return proof, e
	}
	if p.scenario == "copy-crash-after-bytes" {
		if write(p.barrier, map[string]any{"fault": "PROCESS_EXIT_AFTER_DURABLE_TARGET_GET_BEFORE_VERIFY_FACT", "proof": proof}) == nil {
			os.Exit(96)
		}
	}
	return proof, nil
}
