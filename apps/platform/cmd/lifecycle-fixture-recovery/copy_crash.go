package main

import (
 "context"
 "os"
 lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
 "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)
type copyCrashPort struct { lc.CopyRecoveryPort; scenario,barrier string }
func(p copyCrashPort) DiscoverCopyTarget(ctx context.Context,op domain.CopyOperation)(lc.CopyDiscoveryProof,error){
 proof,e:=p.CopyRecoveryPort.DiscoverCopyTarget(ctx,op);if e!=nil{return proof,e}
 if p.scenario=="copy-crash-after-lookup" {
  if write(p.barrier,map[string]any{"fault":"PROCESS_EXIT_AFTER_DURABLE_COPY_LOOKUP_BEFORE_BIND","inventory_call_id":proof.InventoryCallID,"stat_call_id":proof.StatCallID,"exact_target":proof.Copy})==nil{os.Exit(92)}
 }
 return proof,nil
}
