package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

// CheckCustodyPrivatePath requires operator-provisioned private storage; it
// never fixes ACLs, follows junctions, or creates an output directory implicitly.
func CheckCustodyPrivatePath(name string, directory bool) error {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return lc.ErrCustodyScope
	}
	for p := name; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return lc.ErrCustodyScope
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	info, err := os.Lstat(name)
	if err != nil || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return lc.ErrCustodyScope
	}
	return custodyPrivateACL(name, info)
}

func privateCustodyRead(name string, limit int64) ([]byte, error) {
	if CheckCustodyPrivatePath(name, false) != nil {
		return nil, lc.ErrCustodyScope
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, lc.ErrCustodyScope
	}
	b, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	err = errors.Join(readErr, f.Close())
	if err != nil || int64(len(b)) > limit {
		return nil, lc.ErrCustodyScope
	}
	return b, nil
}

// ReadCustodyPrivateFile is shared only by the private CLI's reviewed profile
// and credential-file composition. No file bytes are logged on errors.
func ReadCustodyPrivateFile(name string, limit int64) ([]byte, error) {
	return privateCustodyRead(name, limit)
}

func custodyDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// ExportOperatorCustody runs only after a human authorizes the reviewed plan.
// It owns local output, not source objects or protected Core locations.
func ExportOperatorCustody(ctx context.Context, s *lc.OperatorCustody, p lc.CustodyPlan, key string, reader lc.CustodyReader) (string, error) {
	if s == nil || reader == nil || !s.MatchesPlan(p) || p.Validate() != nil || CheckCustodyPrivatePath(p.OutputDirectory, true) != nil {
		return "", lc.ErrCustodyScope
	}
	var manifestHash string
	err := s.WithOwner(ctx, func(ctx context.Context) error {
		replay, err := s.Prepare(ctx, key)
		if err != nil {
			return err
		}
		manifest := []map[string]any{
			{"versionId": p.First.VersionID, "file": filepath.Join(p.OutputDirectory, p.First.VersionID.String()+".csv")},
			{"versionId": p.Second.VersionID, "file": filepath.Join(p.OutputDirectory, p.Second.VersionID.String()+".csv")},
		}
		expectedManifest, _ := json.MarshalIndent(manifest, "", "  ")
		expectedOwner, _ := json.Marshal(map[string]any{"acquisitionId": s.ID(), "workspaceId": p.WorkspaceID, "runId": p.RunID, "planSha256": p.PlanSHA256})
		verify := func() error {
			owner, e := privateCustodyRead(filepath.Join(p.OutputDirectory, "acquisition.json"), 4096)
			if e != nil || !bytes.Equal(owner, expectedOwner) {
				return lc.ErrCustodyScope
			}
			for _, target := range []lc.CustodyTarget{p.First, p.Second} {
				b, e := privateCustodyRead(filepath.Join(p.OutputDirectory, target.VersionID.String()+".csv"), target.Bytes)
				if e != nil || int64(len(b)) != target.Bytes || custodyDigest(b) != target.SHA256 {
					return lc.ErrCustodyScope
				}
			}
			b, e := privateCustodyRead(filepath.Join(p.OutputDirectory, "custody.json"), 64<<10)
			if e != nil || !bytes.Equal(b, expectedManifest) {
				return lc.ErrCustodyScope
			}
			manifestHash = custodyDigest(b)
			return nil
		}
		if replay {
			if err = verify(); err != nil {
				return err
			}
			h, e := s.AcceptedManifest(ctx)
			if e != nil || h != manifestHash {
				return lc.ErrCustodyScope
			}
			return nil
		}
		// A synchronously returned read has no surviving I/O. Missing durable
		// observations still prevent termination inside Core (UNKNOWN pin).
		defer func() {
			if err != nil {
				finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				_ = s.Finish(finishCtx, false, "")
			}
		}()
		entries, e := os.ReadDir(p.OutputDirectory)
		if e != nil || len(entries) != 0 {
			err = lc.ErrCustodyScope
			return err
		}
		root, e := os.OpenRoot(p.OutputDirectory)
		if e != nil {
			err = lc.ErrCustodyScope
			return err
		}
		defer root.Close()
		original, e := os.Stat(p.OutputDirectory)
		if e != nil {
			err = lc.ErrCustodyScope
			return err
		}
		// Reserve the private directory before any provider call. This excludes
		// even an accidentally overlapping operator plan for another workspace.
		ownerFile, e := root.OpenFile("acquisition.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			err = lc.ErrCustodyScope
			return err
		}
		_, e = ownerFile.Write(expectedOwner)
		if e == nil {
			e = ownerFile.Sync()
		}
		e = errors.Join(e, ownerFile.Close())
		if e != nil {
			err = lc.ErrCustodyScope
			return err
		}
		publish := func(name string, b []byte) error {
			partial := "partial-" + s.ID().String() + "-" + name
			f, e := root.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return lc.ErrCustodyScope
			}
			_, e = f.Write(b)
			if e == nil {
				e = f.Sync()
			}
			e = errors.Join(e, f.Close())
			if e != nil {
				return lc.ErrCustodyScope
			}
			current, e := os.Stat(p.OutputDirectory)
			if e != nil || !os.SameFile(original, current) || CheckCustodyPrivatePath(p.OutputDirectory, true) != nil {
				return lc.ErrCustodyScope
			}
			// Go 1.24 Root has no Link. The operator-private directory and Core
			// owner lock exclude other writers; recheck identity before using Link.
			if e = os.Link(filepath.Join(p.OutputDirectory, partial), filepath.Join(p.OutputDirectory, name)); e != nil {
				return lc.ErrCustodyScope
			}
			if runtime.GOOS != "windows" {
				dir, e := root.Open(".")
				if e != nil {
					return e
				}
				e = errors.Join(dir.Sync(), dir.Close())
				if e != nil {
					return e
				}
			}
			return CheckCustodyPrivatePath(filepath.Join(p.OutputDirectory, name), false)
		}
		for i, target := range []lc.CustodyTarget{p.First, p.Second} {
			var b []byte
			b, err = reader.ReadCustody(ctx, i+1, target, s)
			if err != nil {
				return err
			}
			if int64(len(b)) != target.Bytes || custodyDigest(b) != target.SHA256 {
				err = lc.ErrCustodyScope
				return err
			}
			err = publish(target.VersionID.String()+".csv", b)
			if err != nil {
				return err
			}
		}
		err = publish("custody.json", expectedManifest)
		if err != nil {
			return err
		}
		err = verify()
		if err != nil {
			return err
		}
		err = s.Finish(ctx, true, manifestHash)
		return err
	})
	if err != nil {
		return "", lc.ErrCustodyInterrupted
	}
	return manifestHash, nil
}
