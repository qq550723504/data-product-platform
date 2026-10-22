package pilot

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestCredentialOwnerClosureRejectsUnsignedStaleForeignOpenAndPartial(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	at := time.Now().UTC()
	subjects := []CredentialStopSubject{{Operation: uuid.New(), Copy: uuid.New(), DenialCall: uuid.New()}}
	b, _ := json.Marshal(subjects)
	h := sha256.Sum256(b)
	identity := CredentialOwnerIdentity{Run: uuid.New(), Workspace: uuid.New(), RuntimeClosureSHA256: strings.Repeat("a", 64), SubjectsSHA256: hex.EncodeToString(h[:])}
	proof := CredentialOwnerStop{CredentialOwnerIdentity: identity, Nonce: uuid.New(), ObservedAt: at, OwnerExited: true, ConfigDisabled: true, ParentDisabled: true, ClosedCgroup: true, Subjects: subjects}
	validate := func(p CredentialOwnerStop) error {
		b, _ := json.Marshal(p)
		_, e := (CredentialStopOwner{Key: pub, Expected: identity, Proof: SignedClosedReferenceExecutor{Body: b, Signature: ed25519.Sign(key, b)}}).Validate(at)
		return e
	}
	if e := validate(proof); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*CredentialOwnerStop){"live": func(p *CredentialOwnerStop) { p.OwnerExited = false }, "config": func(p *CredentialOwnerStop) { p.ConfigDisabled = false }, "parent": func(p *CredentialOwnerStop) { p.ParentDisabled = false }, "cgroup": func(p *CredentialOwnerStop) { p.ClosedCgroup = false }, "stale": func(p *CredentialOwnerStop) { p.ObservedAt = at.Add(-61 * time.Second) }, "foreign": func(p *CredentialOwnerStop) { p.Workspace = uuid.New() }, "partial": func(p *CredentialOwnerStop) { p.Subjects = nil }} {
		t.Run(name, func(t *testing.T) {
			p := proof
			mutate(&p)
			if validate(p) == nil {
				t.Fatal("unsafe owner proof accepted")
			}
		})
	}
	b, _ = json.Marshal(proof)
	sig := ed25519.Sign(key, b)
	sig[0] ^= 1
	if _, e := (CredentialStopOwner{Key: pub, Expected: identity, Proof: SignedClosedReferenceExecutor{Body: b, Signature: sig}}).Validate(at); e == nil {
		t.Fatal("tampered signature accepted")
	}
}
