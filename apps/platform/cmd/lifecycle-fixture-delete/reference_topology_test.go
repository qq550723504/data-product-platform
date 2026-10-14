package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"strings"
	"testing"
	"time"
)

func TestReferenceTopologyRequiresExactHashSignatureFreshSourceAndRootFreeHelper(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	at := time.Now().UTC()
	digest := strings.Repeat("a", 64)
	c := referenceConfig{AttestationPublicKey: hex.EncodeToString(pub)}
	c.Fixture.PostgresDSN = "private-test-dsn"
	dsn := sha256.Sum256([]byte(c.Fixture.PostgresDSN))
	topology := referenceTopology{Execution: pilot.ReferenceExecution, CoreID: "c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0", MinioID: "0a344aa06c7bc9c59e5b5ffe8d8d5584e22abf1daeb8b28c549a9735f5b6bf65", OriginalGatewayID: "ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4", GatewayID: digest, GatewayName: "dpp-lifecycle307-5d2274ee-reference-delete-gateway", GatewayImage: "sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f", GatewayConfigSHA256: digest, NetworkID: "33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c", DSNSHA256: hex.EncodeToString(dsn[:]), BinarySHA256: digest}
	raw, _ := json.Marshal(topology)
	hash := sha256.Sum256(raw)
	p := referencePermission{BinarySHA256: digest}
	p.TopologySHA256 = hex.EncodeToString(hash[:])
	witness := referenceTopologyWitness{Kind: "reference-send-topology-v1", CheckedAt: at.Add(-time.Second), TopologySHA256: p.TopologySHA256, HelperID: digest, HelperName: "dpp-lifecycle307-5d2274ee-reference-send-a", NetworkMode: "container:" + topology.GatewayID, Topology: topology}
	sign := func(w referenceTopologyWitness) pilot.SignedClosedReferenceExecutor {
		b, _ := json.Marshal(w)
		return pilot.SignedClosedReferenceExecutor{Body: b, Signature: ed25519.Sign(key, b)}
	}
	if e := validateReferenceTopology(c, p, raw, sign(witness), at); e != nil {
		t.Fatal(e)
	}
	changed := p
	changed.TopologySHA256 = strings.Repeat("b", 64)
	if validateReferenceTopology(c, changed, raw, sign(witness), at) == nil {
		t.Fatal("arbitrary 64-character topology accepted")
	}
	for _, mutate := range []func(*referenceTopologyWitness){func(w *referenceTopologyWitness) { w.Topology.CoreID = digest }, func(w *referenceTopologyWitness) { w.Topology.GatewayID = w.Topology.OriginalGatewayID }, func(w *referenceTopologyWitness) { w.CheckedAt = at.Add(-61 * time.Second) }, func(w *referenceTopologyWitness) { w.CheckedAt = at.Add(time.Second) }, func(w *referenceTopologyWitness) { w.NetworkMode = "host" }, func(w *referenceTopologyWitness) { w.HelperName = "foreign" }} {
		w := witness
		mutate(&w)
		if validateReferenceTopology(c, p, raw, sign(w), at) == nil {
			t.Fatal("foreign/stale topology accepted")
		}
	}
	signed := sign(witness)
	signed.Body[0] = '['
	if validateReferenceTopology(c, p, raw, signed, at) == nil {
		t.Fatal("tampered topology accepted")
	}
}
func TestReferencePolicyAndGatewayExcludeProtectedOutputsAndBareDelete(t *testing.T) {
	var policy struct {
		Statement []struct {
			Action    []string
			Resource  []string
			Condition map[string]map[string]string
		}
	}
	if json.Unmarshal(referenceIAMPolicy(), &policy) != nil || len(policy.Statement) != 2 {
		t.Fatal("expanded policy")
	}
	for _, s := range policy.Statement {
		if len(s.Action) != 1 || s.Action[0] != "s3:DeleteObjectVersion" || s.Condition["StringEquals"]["s3:versionid"] == "" || len(s.Resource) != 1 {
			t.Fatal("unversioned/broad policy")
		}
	}
	active := referenceGateway("refdelete614cd9ad", "inspectfixture", true)
	sealed := referenceGateway("refdelete614cd9ad", "inspectfixture", false)
	for _, c := range pilot.ReferenceTargets() {
		if !strings.Contains(active, "{http.request.uri} == '/"+c.Namespace+"/"+c.Key+"?versionId="+c.ProviderVersion+"'") {
			t.Fatal("missing complete exact URI")
		}
	}
	if strings.Contains(sealed, "@approvedDelete") || !strings.Contains(sealed, "@mutate method DELETE PUT POST PATCH") || !strings.Contains(active, "handle_response @accepted") || !strings.Contains(active, "abort") {
		t.Fatal("sealed/fault contract missing")
	}
}
