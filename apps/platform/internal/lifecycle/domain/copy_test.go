package domain

import (
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestRestoreCheckpointChainRejectsMissingGapForeignAndTamper(t *testing.T) {
	at := time.Now().UTC()
	w, v, e := uuid.New(), uuid.New(), uuid.New()
	a := RestoreCheckpoint{ID: uuid.New(), WorkspaceID: w, DatasetVersionID: v, Epoch: e, Sequence: 1, DependencyRevision: 2, StateSHA256: strings.Repeat("a", 64), PreviousSHA256: strings.Repeat("0", 64), CreatedAt: at, ExpiresAt: at.Add(time.Minute)}
	a.ContentSHA256 = a.Hash()
	b := a
	b.ID = uuid.New()
	b.Sequence = 2
	b.DependencyRevision = 3
	b.PreviousSHA256 = a.ContentSHA256
	b.ContentSHA256 = b.Hash()
	if ValidateCheckpointChain([]RestoreCheckpoint{a, b}, w, v, e) != nil || b.Validate(at) != nil {
		t.Fatal("valid chain rejected")
	}
	for _, cuts := range [][]RestoreCheckpoint{nil, {b}, {a, a}, {a, func() RestoreCheckpoint { x := b; x.StateSHA256 = strings.Repeat("c", 64); return x }()}} {
		if ValidateCheckpointChain(cuts, w, v, e) == nil {
			t.Fatal("invalid chain admitted")
		}
	}
	if ValidateCheckpointChain([]RestoreCheckpoint{a, b}, uuid.New(), v, e) == nil || b.Validate(at.Add(time.Minute)) == nil {
		t.Fatal("foreign/expired witness admitted")
	}
}
