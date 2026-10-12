package infrastructure

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func ledgerFixture(t *testing.T) (FileRestoreLedger, domain.RestoreCheckpoint, domain.RestoreCheckpoint) {
	t.Helper()
	l := FileRestoreLedger{Path: filepath.Join(t.TempDir(), "witness.jsonl"), WorkspaceID: uuid.New(), DatasetVersionID: uuid.New(), Epoch: uuid.New()}
	c := domain.RestoreCheckpoint{ID: uuid.New(), WorkspaceID: l.WorkspaceID, DatasetVersionID: l.DatasetVersionID, Epoch: l.Epoch, Sequence: 1, DependencyRevision: 1, StateSHA256: strings.Repeat("a", 64), PreviousSHA256: strings.Repeat("0", 64), CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	c.ExpiresAt = c.CreatedAt.Add(time.Hour)
	c.ContentSHA256 = c.Hash()
	n := c
	n.ID, n.Sequence, n.DependencyRevision, n.PreviousSHA256 = uuid.New(), 2, 2, c.ContentSHA256
	n.CreatedAt, n.ExpiresAt = c.CreatedAt.Add(time.Second), c.ExpiresAt.Add(time.Second)
	n.ContentSHA256 = n.Hash()
	return l, c, n
}

func TestRestoreLedgerExactReplayAndOutOfOrderRetry(t *testing.T) {
	l, first, next := ledgerFixture(t)
	if e := l.Append(next); e == nil {
		t.Fatal("out-of-order publication must remain rejected")
	}
	for _, c := range []domain.RestoreCheckpoint{first, first, next, first, next} {
		if e := l.Append(c); e != nil {
			t.Fatal(e)
		}
	}
	cuts, e := l.read()
	if e != nil || len(cuts) != 2 || cuts[0].ID != first.ID || cuts[1].ID != next.ID {
		t.Fatal("replays must not duplicate or replace cuts", cuts, e)
	}
	wrong := first
	wrong.StateSHA256 = strings.Repeat("b", 64)
	wrong.ContentSHA256 = wrong.Hash()
	before, _ := os.ReadFile(l.Path)
	if e := l.Append(wrong); e == nil {
		t.Fatal("same sequence with changed content must fail")
	}
	after, _ := os.ReadFile(l.Path)
	if string(before) != string(after) {
		t.Fatal("conflict changed independent history")
	}
}

func TestRestoreLedgerCompletesOnlyExactInterruptedAppend(t *testing.T) {
	for _, initial := range []bool{true, false} {
		t.Run(map[bool]string{true: "first", false: "next"}[initial], func(t *testing.T) {
			l, first, next := ledgerFixture(t)
			cut := first
			var prefix []byte
			if !initial {
				if e := l.Append(first); e != nil {
					t.Fatal(e)
				}
				prefix, _ = os.ReadFile(l.Path)
				cut = next
			}
			line, _ := json.Marshal(cut)
			torn := append(append([]byte{}, prefix...), line[:len(line)/2]...)
			if e := os.WriteFile(l.Path, torn, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := l.Latest(context.Background()); e == nil {
				t.Fatal("partial witness must not authorize a restore")
			}
			wrong := cut
			wrong.ID = uuid.New()
			wrong.ContentSHA256 = wrong.Hash()
			if e := l.Append(wrong); e == nil {
				t.Fatal("different cut cannot complete a partial record")
			}
			unchanged, _ := os.ReadFile(l.Path)
			if string(unchanged) != string(torn) {
				t.Fatal("failed recovery changed prefix")
			}
			if e := l.Append(cut); e != nil {
				t.Fatal(e)
			}
			complete, _ := os.ReadFile(l.Path)
			want := append(append(prefix, line...), '\n')
			if string(complete) != string(want) {
				t.Fatal("exact append continuation changed prior bytes")
			}
			latest, e := l.Latest(context.Background())
			if e != nil || latest.ID != cut.ID {
				t.Fatal(latest, e)
			}
		})
	}
}

func TestRestoreLedgerConcurrentReplayAcrossProcesses(t *testing.T) {
	if path := os.Getenv("TEST_LEDGER_REPLAY_PATH"); path != "" {
		var cut domain.RestoreCheckpoint
		if e := json.Unmarshal([]byte(os.Getenv("TEST_LEDGER_REPLAY_CUT")), &cut); e != nil {
			t.Fatal(e)
		}
		l := FileRestoreLedger{Path: path, WorkspaceID: cut.WorkspaceID, DatasetVersionID: cut.DatasetVersionID, Epoch: cut.Epoch}
		if e := l.Append(cut); e != nil {
			t.Fatal(e)
		}
		return
	}
	l, cut, _ := ledgerFixture(t)
	b, _ := json.Marshal(cut)
	children := make([]*exec.Cmd, 8)
	for i := range children {
		children[i] = exec.Command(os.Args[0], "-test.run=^TestRestoreLedgerConcurrentReplayAcrossProcesses$")
		children[i].Env = append(os.Environ(), "TEST_LEDGER_REPLAY_PATH="+l.Path, "TEST_LEDGER_REPLAY_CUT="+string(b))
		if e := children[i].Start(); e != nil {
			t.Fatal(e)
		}
	}
	for _, child := range children {
		if e := child.Wait(); e != nil {
			t.Fatal(e)
		}
	}
	cuts, e := l.read()
	if e != nil || len(cuts) != 1 || cuts[0].ID != cut.ID {
		t.Fatal("cross-process replays corrupted the chain", cuts, e)
	}
}
