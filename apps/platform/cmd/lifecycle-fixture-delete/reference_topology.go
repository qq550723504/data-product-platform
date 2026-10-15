package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

// Stable topology is bound to permission; fresh signed host observations bind
// the current root-free container, network namespace, configuration and binary.
type referenceTopology struct {
	Execution, CoreID, MinioID, OriginalGatewayID, GatewayID, GatewayName, GatewayImage, GatewayConfigSHA256, NetworkID, DSNSHA256, BinarySHA256 string
}
type referenceTopologyWitness struct {
	Kind                                              string
	CheckedAt                                         time.Time
	TopologySHA256, HelperID, HelperName, NetworkMode string
	Topology                                          referenceTopology
}

func validReferenceTopology(t referenceTopology) bool {
	return t.Execution == pilot.ReferenceExecution && t.CoreID == "c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0" && t.MinioID == "0a344aa06c7bc9c59e5b5ffe8d8d5584e22abf1daeb8b28c549a9735f5b6bf65" && t.OriginalGatewayID == "ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4" && t.GatewayName == referenceGatewayName && len(t.GatewayID) == 64 && t.GatewayID != t.OriginalGatewayID && t.GatewayImage == "sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f" && t.NetworkID == "33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c" && len(t.GatewayConfigSHA256) == 64 && len(t.DSNSHA256) == 64 && len(t.BinarySHA256) == 64
}
func validateReferenceTopology(c referenceConfig, p referencePermission, raw []byte, signed pilot.SignedClosedReferenceExecutor, at time.Time) error {
	key, e := hex.DecodeString(c.AttestationPublicKey)
	var t referenceTopology
	var witness referenceTopologyWitness
	if e != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, signed.Body, signed.Signature) || json.Unmarshal(raw, &t) != nil || json.Unmarshal(signed.Body, &witness) != nil || !validReferenceTopology(t) {
		return lc.ErrStorageDenied
	}
	hash := sha256.Sum256(raw)
	dsn := sha256.Sum256([]byte(c.Fixture.PostgresDSN))
	if hex.EncodeToString(hash[:]) != p.TopologySHA256 || witness.TopologySHA256 != p.TopologySHA256 || witness.Topology != t || t.BinarySHA256 != p.BinarySHA256 || t.DSNSHA256 != hex.EncodeToString(dsn[:]) || witness.Kind != "reference-send-topology-v1" || witness.CheckedAt.IsZero() || at.Before(witness.CheckedAt) || at.Sub(witness.CheckedAt) > time.Minute || len(witness.HelperID) != 64 || witness.NetworkMode != "container:"+t.GatewayID || (witness.HelperName != referenceSenderPrefix+"a" && witness.HelperName != referenceSenderPrefix+"b") {
		return lc.ErrStorageDenied
	}
	return nil
}
func referenceHostTopology(ctx context.Context, stage string) error {
	// Fixed host entry point, never a client-submitted topology/signing service.
	base := `C:\Users\Henry\.codex\fixture-private\dpp-lifecycle307-5d2274ee\reference-disposal-execution02`
	folder := filepath.Join(base, "executor-"+stage)
	var c referenceConfig
	var p referencePermission
	if read(filepath.Join(folder, "reference.json"), &c) != nil || read(filepath.Join(folder, "reference-permission.json"), &p) != nil || validateReferencePermission(p, c, time.Now().UTC()) != nil {
		return lc.ErrStorageDenied
	}
	raw, e := os.ReadFile(filepath.Join(folder, "reference-topology.json"))
	var t referenceTopology
	if e != nil || json.Unmarshal(raw, &t) != nil || !validReferenceTopology(t) {
		return lc.ErrStorageScope
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != p.TopologySHA256 {
		return lc.ErrStorageScope
	}
	dsn, e := url.Parse(c.Fixture.PostgresDSN)
	if e != nil || dsn.Hostname() != "dpp-lifecycle307-5d2274ee-postgres" || dsn.Port() != "5432" || dsn.Path != "/lifecycle307" {
		return lc.ErrStorageScope
	}
	get := func(name string) (map[string]json.RawMessage, error) {
		data, e := exec.CommandContext(ctx, "docker", "inspect", name).Output()
		if e != nil {
			return nil, lc.ErrStorageScope
		}
		var objects []map[string]json.RawMessage
		if json.Unmarshal(data, &objects) != nil || len(objects) != 1 {
			return nil, lc.ErrStorageScope
		}
		return objects[0], nil
	}
	for name, id := range map[string]string{"dpp-lifecycle307-5d2274ee-postgres": t.CoreID, "dpp-lifecycle307-5d2274ee-minio": t.MinioID, "dpp-lifecycle307-5d2274ee-gateway": t.OriginalGatewayID, t.GatewayName: t.GatewayID} {
		item, e := get(name)
		if e != nil {
			return e
		}
		var found string
		var state struct{ Running bool }
		var host struct{ PortBindings map[string]any }
		var network struct {
			Networks map[string]struct{ NetworkID string }
		}
		if json.Unmarshal(item["Id"], &found) != nil || found != id || json.Unmarshal(item["State"], &state) != nil || !state.Running || json.Unmarshal(item["HostConfig"], &host) != nil || len(host.PortBindings) != 0 || json.Unmarshal(item["NetworkSettings"], &network) != nil || len(network.Networks) != 1 {
			return lc.ErrStorageScope
		}
		for _, n := range network.Networks {
			if n.NetworkID != t.NetworkID {
				return lc.ErrStorageScope
			}
		}
		if name == t.GatewayName {
			var image string
			if json.Unmarshal(item["Image"], &image) != nil || image != t.GatewayImage {
				return lc.ErrStorageScope
			}
		}
	}
	original, e := os.ReadFile(filepath.Join(filepath.Dir(base), "Caddyfile"))
	if e != nil {
		return e
	}
	originalHash := sha256.Sum256(original)
	if hex.EncodeToString(originalHash[:]) != "b301fc4fe59f64ebe1e11f1488dde521b774622641ccc8377fe41cb64e839e80" {
		return lc.ErrStorageDenied
	}
	gateway, e := os.ReadFile(filepath.Join(base, "gateway", "Caddyfile"))
	if e != nil {
		return e
	}
	gatewayHash := sha256.Sum256(gateway)
	if hex.EncodeToString(gatewayHash[:]) != t.GatewayConfigSHA256 || string(gateway) != referenceGateway(p.DeleteAccess, c.Fixture.InspectAccess, true) {
		return lc.ErrStorageScope
	}
	binary, e := os.ReadFile(filepath.Join(folder, "executor"))
	if e != nil {
		return e
	}
	binaryHash := sha256.Sum256(binary)
	if hex.EncodeToString(binaryHash[:]) != p.BinarySHA256 {
		return lc.ErrStorageScope
	}
	name := referenceSenderPrefix + stage
	helper, e := get(name)
	if e != nil {
		return e
	}
	var id string
	var host struct{ NetworkMode string }
	var state struct {
		Status  string
		Running bool
	}
	var mounts []struct {
		Type, Source, Destination string
		RW                        bool
	}
	var config struct {
		Entrypoint, Cmd []string
		Env             []string
	}
	if json.Unmarshal(helper["Id"], &id) != nil || json.Unmarshal(helper["State"], &state) != nil || state.Status != "created" || state.Running || json.Unmarshal(helper["HostConfig"], &host) != nil || host.NetworkMode != "container:"+t.GatewayID || json.Unmarshal(helper["Mounts"], &mounts) != nil || len(mounts) != 2 || json.Unmarshal(helper["Config"], &config) != nil || len(config.Entrypoint) != 1 || config.Entrypoint[0] != "/run/private/executor" || len(config.Cmd) != 0 {
		return lc.ErrStorageScope
	}
	for _, mount := range mounts {
		if mount.Type != "bind" || (mount.Destination != "/run/private" && mount.Destination != "/run/output") || mount.RW != (mount.Destination == "/run/output") || (mount.Destination == "/run/private" && filepath.Clean(mount.Source) != filepath.Clean(folder)) {
			return lc.ErrStorageScope
		}
	}
	allowedEnv := map[string]bool{"PATH": true, "GOSU_VERSION": true, "LANG": true, "PG_MAJOR": true, "PG_VERSION": true, "PG_SHA256": true, "DOCKER_PG_LLVM_DEPS": true, "PGDATA": true, "LIFECYCLE_REFERENCE_PROFILE": true, "LIFECYCLE_REFERENCE_MODE": true}
	modeOK, profileOK := false, false
	for _, entry := range config.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || !allowedEnv[parts[0]] {
			return lc.ErrStorageScope
		}
		modeOK = modeOK || entry == "LIFECYCLE_REFERENCE_MODE=send-"+stage
		profileOK = profileOK || entry == "LIFECYCLE_REFERENCE_PROFILE="+referenceProfile
	}
	if !modeOK || !profileOK {
		return lc.ErrStorageScope
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	body, _ := json.Marshal(referenceTopologyWitness{Kind: "reference-send-topology-v1", CheckedAt: time.Now().UTC(), TopologySHA256: p.TopologySHA256, HelperID: id, HelperName: name, NetworkMode: host.NetworkMode, Topology: t})
	signed := pilot.SignedClosedReferenceExecutor{Body: body, Signature: ed25519.Sign(key, body)}
	c.AttestationPublicKey = hex.EncodeToString(pub)
	encoded, _ := json.MarshalIndent(c, "", "  ")
	if e = os.WriteFile(filepath.Join(folder, "reference.json"), append(encoded, '\n'), 0600); e != nil {
		return e
	}
	return write(filepath.Join(folder, "current-topology.json"), signed)
}
