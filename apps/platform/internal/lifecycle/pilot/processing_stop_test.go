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

func TestProcessingStopTrustRejectsForeignOpenStaleAndTamperedProof(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	at := time.Now().UTC()
	h := strings.Repeat("a", 64)
	identity := ProcessingOwnerIdentity{Run: uuid.New(), Workspace: uuid.New(), Execution: uuid.New(), HelperID: h, ImageSHA256: h, BinarySHA256: h, PinsSHA256: h, RuntimeClosureSHA256: h}
	p := ProcessingOwnerStop{ProcessingOwnerIdentity: identity, Nonce: uuid.New(), ObservedAt: at, Exited: true, RestartFenced: true, CredentialsDisabled: true, ConfigDisabled: true, ClosedCgroup: true}
	verify := func(p ProcessingOwnerStop) error {
		b, _ := json.Marshal(p)
		v := ProcessingStopVerifier{Key: pub, Expected: identity, Proof: SignedClosedReferenceExecutor{Body: b, Signature: ed25519.Sign(key, b)}}
		_, e := v.Validate(at)
		return e
	}
	if e := verify(p); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*ProcessingOwnerStop){"live": func(x *ProcessingOwnerStop) { x.Exited = false }, "restart": func(x *ProcessingOwnerStop) { x.RestartFenced = false }, "credentials": func(x *ProcessingOwnerStop) { x.CredentialsDisabled = false }, "config": func(x *ProcessingOwnerStop) { x.ConfigDisabled = false }, "cgroup": func(x *ProcessingOwnerStop) { x.ClosedCgroup = false }, "stale": func(x *ProcessingOwnerStop) { x.ObservedAt = at.Add(-61 * time.Second) }, "future": func(x *ProcessingOwnerStop) { x.ObservedAt = at.Add(time.Second) }, "foreign": func(x *ProcessingOwnerStop) { x.Workspace = uuid.New() }, "membership": func(x *ProcessingOwnerStop) { x.PinsSHA256 = strings.Repeat("b", 64) }} {
		t.Run(name, func(t *testing.T) {
			x := p
			mutate(&x)
			if verify(x) == nil {
				t.Fatal("unsafe owner accepted")
			}
		})
	}
	b, _ := json.Marshal(p)
	sig := ed25519.Sign(key, b)
	sig[0] ^= 1
	if _, e := (ProcessingStopVerifier{Key: pub, Expected: identity, Proof: SignedClosedReferenceExecutor{Body: b, Signature: sig}}).Validate(at); e == nil {
		t.Fatal("tampered signature accepted")
	}
	var round SignedClosedReferenceExecutor
	raw, _ := json.MarshalIndent(SignedClosedReferenceExecutor{Body: b, Signature: ed25519.Sign(key, b)}, "", "  ")
	_ = json.Unmarshal(raw, &round)
	if _, e := (ProcessingStopVerifier{Key: pub, Expected: identity, Proof: round}).Validate(at); e != nil {
		t.Fatal("signed bytes changed across JSON", e)
	}
}
