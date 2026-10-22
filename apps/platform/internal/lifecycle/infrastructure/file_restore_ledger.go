package infrastructure

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"io"
	"os"
	"path/filepath"
)

// Fixture-only trusted publisher/witness; no network receipt ingestion, client
// chosen path, or recovery from the old Core backup. Keep this private file off
// Core/provider volumes and outside the backup scope.
type FileRestoreLedger struct {
	Path                                 string
	WorkspaceID, DatasetVersionID, Epoch uuid.UUID
}

func (l FileRestoreLedger) read() ([]domain.RestoreCheckpoint, error) {
	if !filepath.IsAbs(l.Path) {
		return nil, domain.ErrRestoreLedger
	}
	f, e := os.Open(l.Path)
	if e != nil {
		return nil, domain.ErrRestoreLedger
	}
	defer f.Close()
	unlock, e := lockRestoreLedger(f, false)
	if e != nil {
		return nil, domain.ErrRestoreLedger
	}
	defer unlock()
	b, e := io.ReadAll(io.LimitReader(f, 256<<10+1))
	if e != nil {
		return nil, domain.ErrRestoreLedger
	}
	return l.decode(b)
}

func (l FileRestoreLedger) decode(b []byte) ([]domain.RestoreCheckpoint, error) {
	if len(b) > 256<<10 || len(b) == 0 || b[len(b)-1] != '\n' {
		return nil, domain.ErrRestoreLedger
	}
	var cuts []domain.RestoreCheckpoint
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1024), 64<<10)
	for sc.Scan() {
		var c domain.RestoreCheckpoint
		d := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		d.DisallowUnknownFields()
		if d.Decode(&c) != nil {
			return nil, domain.ErrRestoreLedger
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return nil, domain.ErrRestoreLedger
		}
		cuts = append(cuts, c)
	}
	if sc.Err() != nil || domain.ValidateCheckpointChain(cuts, l.WorkspaceID, l.DatasetVersionID, l.Epoch) != nil {
		return nil, domain.ErrRestoreLedger
	}
	return cuts, nil
}
func (l FileRestoreLedger) Latest(_ context.Context) (domain.RestoreCheckpoint, error) {
	cuts, e := l.read()
	if e != nil {
		return domain.RestoreCheckpoint{}, e
	}
	return cuts[len(cuts)-1], nil
}
func (l FileRestoreLedger) Append(c domain.RestoreCheckpoint) error {
	if !filepath.IsAbs(l.Path) || c.Validate(c.CreatedAt) != nil || c.WorkspaceID != l.WorkspaceID || c.DatasetVersionID != l.DatasetVersionID || c.Epoch != l.Epoch {
		return domain.ErrRestoreLedger
	}
	// OS file locks serialize cooperating publishers across processes, including
	// same-cut replay. No Core transaction or delivery fence covers file I/O.
	f, e := os.OpenFile(l.Path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	unlock, e := lockRestoreLedger(f, true)
	if e != nil {
		return e
	}
	defer unlock()
	data, e := io.ReadAll(io.LimitReader(f, 256<<10+1))
	if e != nil || len(data) > 256<<10 {
		return domain.ErrRestoreLedger
	}
	complete := bytes.LastIndexByte(data, '\n') + 1
	var cuts []domain.RestoreCheckpoint
	if complete > 0 {
		cuts, e = l.decode(data[:complete])
		if e != nil {
			return e
		}
	}
	if complete == len(data) && c.Sequence <= int64(len(cuts)) {
		old := cuts[c.Sequence-1]
		if old.ID != c.ID || old.ContentSHA256 != c.ContentSHA256 {
			return domain.ErrRestoreLedger
		}
		// Retry Sync too: a prior append can have written all bytes before Sync
		// failed. Never append a duplicate checkpoint or alter its expiry.
		return f.Sync()
	}
	if domain.ValidateCheckpointChain(append(cuts, c), l.WorkspaceID, l.DatasetVersionID, l.Epoch) != nil {
		return domain.ErrRestoreLedger
	}
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	b = append(b, '\n')
	tail := data[complete:]
	if !bytes.HasPrefix(b, tail) || complete+len(b) > 256<<10 {
		return domain.ErrRestoreLedger
	}
	// Only finish an exact prefix of this already-committed cut after a torn
	// append. Different/truncated history is rejected; no bytes are overwritten.
	if _, e = f.Seek(int64(len(data)), io.SeekStart); e != nil {
		return e
	}
	remaining := b[len(tail):]
	if n, e := f.Write(remaining); e != nil || n != len(remaining) {
		return domain.ErrRestoreLedger
	}
	return f.Sync()
}
