package pilot

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestClosedExecutorSignatureScopeNonceAndFreshness(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	at := time.Now().UTC()
	p := ClosedReferenceExecutor{Nonce: uuid.New(), CheckedAt: at.Add(-time.Second), Environment: ReferenceEnvironment, PlanHash: ReferencePlanHash, HelperID: "fb92bdcd150127c50163448de9d918f3a2d9a18e2384274b30febfaf647bdb87", BinarySHA256: "5e04e88bbd65880a5e65da5795dcdc116021512e04d27a200ec0d03f83e972c8", ScriptSHA256: "24e1d6d19394208016ee1d9dd0ddaa06669880d2cebae03b4c7996db0303fd1c", CandidateSHA256: strings.Repeat("a", 64)}
	sign := func(p ClosedReferenceExecutor) ReferenceNoSendVerifier {
		b, _ := json.Marshal(p)
		return ReferenceNoSendVerifier{pub, SignedClosedReferenceExecutor{b, ed25519.Sign(key, b)}}
	}
	ref := "closed-executor:" + p.Nonce.String()
	if _, e := sign(p).Validate(at, ref); e != nil {
		t.Fatal(e)
	}
	serialized, e := json.MarshalIndent(sign(p).Proof, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	var persisted SignedClosedReferenceExecutor
	if e = json.Unmarshal(serialized, &persisted); e != nil {
		t.Fatal(e)
	}
	if _, e = (ReferenceNoSendVerifier{pub, persisted}).Validate(at, ref); e != nil {
		t.Fatalf("signed proof lost identity across persisted JSON: %v", e)
	}
	mutations := []func(*ClosedReferenceExecutor){func(p *ClosedReferenceExecutor) { p.Environment = "foreign" }, func(p *ClosedReferenceExecutor) { p.HelperID = "replacement" }, func(p *ClosedReferenceExecutor) { p.PlanHash = strings.Repeat("b", 64) }, func(p *ClosedReferenceExecutor) { p.ScriptSHA256 = strings.Repeat("b", 64) }, func(p *ClosedReferenceExecutor) { p.BinarySHA256 = strings.Repeat("b", 64) }, func(p *ClosedReferenceExecutor) { p.CheckedAt = at.Add(-61 * time.Second) }, func(p *ClosedReferenceExecutor) { p.CheckedAt = at.Add(time.Second) }, func(p *ClosedReferenceExecutor) { p.Nonce = uuid.New() }, func(p *ClosedReferenceExecutor) { p.CandidateSHA256 = "bad" }}
	for _, mutate := range mutations {
		changed := p
		mutate(&changed)
		if _, e := sign(changed).Validate(at, ref); e == nil {
			t.Fatal("foreign/stale signed proof allowed")
		}
	}
	bad := sign(p)
	bad.Proof.Body[0] = '['
	if _, e := bad.Validate(at, ref); e == nil {
		t.Fatal("tampered proof allowed")
	}
	bad = sign(p)
	bad.Key, _, _ = ed25519.GenerateKey(rand.Reader)
	if _, e := bad.Validate(at, ref); e == nil {
		t.Fatal("untrusted signer allowed")
	}
}
