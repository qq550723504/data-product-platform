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
	b, e := io.ReadAll(io.LimitReader(f, 256<<10+1))
	if e != nil || len(b) > 256<<10 || len(b) == 0 || b[len(b)-1] != '\n' {
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
	var cuts []domain.RestoreCheckpoint
	if _, e := os.Stat(l.Path); e == nil {
		var err error
		cuts, err = l.read()
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return domain.ErrRestoreLedger
	}
	if domain.ValidateCheckpointChain(append(cuts, c), l.WorkspaceID, l.DatasetVersionID, l.Epoch) != nil {
		return domain.ErrRestoreLedger
	}
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	b = append(b, '\n')
	f, e := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if n, e := f.Write(b); e != nil || n != len(b) {
		return domain.ErrRestoreLedger
	}
	return f.Sync()
}
