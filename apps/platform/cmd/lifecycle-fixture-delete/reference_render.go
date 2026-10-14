package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"os"
	"path/filepath"
)

func writeReferenceRaw(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(data); e != nil {
		return e
	}
	return f.Sync()
}
func renderReference() error {
	c, e := readReferenceConfig()
	if e != nil {
		return e
	}
	active := []byte(referenceGateway("refdelete614cd9ad", c.Fixture.InspectAccess, true))
	sealed := []byte(referenceGateway("refdelete614cd9ad", c.Fixture.InspectAccess, false))
	policy := referenceIAMPolicy()
	binary, e := os.ReadFile(os.Args[0])
	if e != nil {
		return e
	}
	sum := sha256.Sum256(binary)
	for name, data := range map[string][]byte{"gateway-active.Caddyfile": active, "gateway-sealed.Caddyfile": sealed, "delete-executor-policy.json": policy} {
		if e = writeReferenceRaw(filepath.Join(c.Fixture.OutputDirectory, name), data); e != nil {
			return e
		}
	}
	h := func(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
	return write(filepath.Join(c.Fixture.OutputDirectory, "bounded-profile.json"), map[string]any{"status": "IMPLEMENTED_DISABLED", "environment": pilot.ReferenceEnvironment, "execution": pilot.ReferenceExecution, "workspace": pilot.ReferenceWorkspace, "provider": pilot.ReferenceProvider, "incarnation": pilot.ReferenceIncarnation, "plan": pilot.ReferencePlan, "plan_hash": pilot.ReferencePlanHash, "inventory_hash": pilot.ReferenceInventoryHash, "inventory": pilot.ReferenceCopies(), "targets": pilot.ReferenceTargets(), "target_digest": referenceTargetsDigest(), "binary_sha256": hex.EncodeToString(sum[:]), "active_gateway_sha256": h(active), "sealed_gateway_sha256": h(sealed), "iam_policy_sha256": h(policy), "permission_enabled": false, "human_confirmation": "NOT_RECEIVED", "max_total": 2, "max_per_target": 1, "max_minutes": 40, "new_dispatch_only": true, "old_trials_never_sent": true, "inventory_closure": "6 then 5 then 4; only exact absence receipts remove registry members", "fresh_host_topology_required": true, "provider_delete": 0, "iam_native_provision": "NOT_RUN; only after separate exact confirmation", "policy": json.RawMessage(policy)})
}
