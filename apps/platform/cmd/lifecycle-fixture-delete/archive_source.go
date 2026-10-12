package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Two explicitly confirmed local fixture scopes, never arbitrary CLI locators.
func selectProfile() error {
	switch os.Getenv("LIFECYCLE_DELETE_PROFILE") {
	case "":
		return nil
	case "worker-delete-4abcfca5":
		selectWorkerDelete()
		return nil
	case "archive-source-c9f47cab":
		archiveSource = true
		run = "c9f47cab-23c6-4d16-8102-a4a34bac1501"
		workspace = uuid.MustParse("5fe971c0-7682-4b6b-b4c6-741883247365")
		planID = uuid.MustParse("50b71a6c-f837-42e4-a670-c924aebceb22")
		planHash = "7ccf841b84d919b3f9634983fe789e74fb82b0cbbf86017fd1ad95a841aed47e"
		providerID = "24dba412-9b64-46c0-9cc5-ae56a5d3a124"
		incarnationID = "b19427cf-0d61-4417-a5e1-76c7dd894046"
		return nil
	default:
		return errors.New("unknown controlled scope")
	}
}

func archiveCopies(c config) ([]domain.Copy, error) {
	var accepted struct {
		Objects []domain.Copy `json:"objects"`
	}
	if e := read(filepath.Join(c.OutputDirectory, "archive-acceptance.json"), &accepted); e != nil {
		return nil, e
	}
	expected := map[string]string{
		"dpp-lifecycle307-online/fixture/a.csv":             "3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d",
		"dpp-lifecycle307-online/fixture/b.csv":             "8228fe6b-e938-4404-8ad2-4e490e875495",
		"dpp-lifecycle307-quarantine/fixture/a.csv":         "87bd6a39-f420-4bfa-9619-4812a01e5835",
		"dpp-lifecycle307-quarantine/fixture/b.csv":         "6e3b6ae2-2b8e-40ee-9103-419322b78718",
		"dpp-lifecycle307-quarantine/archive/a.csv":         "613c3231-5077-4326-9a7c-4a073a105e47",
		"dpp-lifecycle307-quarantine/restore/online-a.csv":  "81f1f926-ccf9-4df2-8f5b-abce76b2c131",
		"dpp-lifecycle307-quarantine/restore/old-cut-a.csv": "5d85eea5-7451-4a11-a4be-d7b80b39db1d",
		"dpp-lifecycle307-quarantine/restore/missing-a.csv": "76b867ec-a365-47fd-b2ab-11e825c7198e",
		"dpp-lifecycle307-quarantine/restore/gap-a.csv":     "2a316e28-3efb-4e94-ab5b-c1e2637b50df",
	}
	if len(accepted.Objects) != len(expected) {
		return nil, lc.ErrStorageScope
	}
	for _, copy := range accepted.Objects {
		key := copy.Namespace + "/" + copy.Key
		if copy.Validate() != nil || copy.WorkspaceID != workspace || copy.ProviderInstanceID != c.ProviderInstanceID || copy.IncarnationID != c.IncarnationID || expected[key] != copy.ProviderVersion {
			return nil, lc.ErrStorageScope
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		return nil, lc.ErrStorageScope
	}
	return accepted.Objects, nil
}

func containsCopy(copies []domain.Copy, id uuid.UUID) bool {
	for _, c := range copies {
		if c.ID == id {
			return true
		}
	}
	return false
}

func inventoryMatches(i lc.NamespaceInventory, all, absent []domain.Copy) bool {
	expected := map[string]domain.Copy{}
	for _, c := range all {
		if c.Namespace == i.Namespace && !containsCopy(absent, c.ID) {
			expected[c.Key+"\x00"+c.ProviderVersion] = c
		}
	}
	if len(expected) != len(i.Versions) {
		return false
	}
	for _, v := range i.Versions {
		c, ok := expected[v.Key+"\x00"+v.Version]
		if !ok || v.DeleteMarker || v.ByteSize != c.ByteSize || c.WorkspaceID != i.WorkspaceID || c.ProviderInstanceID != i.ProviderInstanceID || c.IncarnationID != i.IncarnationID {
			return false
		}
		delete(expected, v.Key+"\x00"+v.Version)
	}
	return len(expected) == 0
}

func sameProtection(p lc.CopyProtection, before []any) bool {
	for _, item := range before {
		old := item.(map[string]any)["protection"].(lc.CopyProtection)
		if old.Copy == p.Copy {
			if old.LegalHold != p.LegalHold || old.RetentionMode != p.RetentionMode || (old.RetainUntil == nil) != (p.RetainUntil == nil) {
				return false
			}
			return old.RetainUntil == nil || old.RetainUntil.Equal(*p.RetainUntil)
		}
	}
	return false
}

func expectedArchiveProtection(c config, p lc.CopyProtection) error {
	var original manifest
	if e := read(filepath.Join(c.OutputDirectory, "manifest.json"), &original); e != nil {
		return e
	}
	for _, o := range original.Objects {
		if o.CopyID == p.Copy.ID {
			if o.LegalHold != p.LegalHold || o.Retention != p.RetentionMode || (o.RetainUntil == nil) != (p.RetainUntil == nil) {
				return lc.ErrStorageContract
			}
			if o.RetainUntil != nil && !o.RetainUntil.Equal(*p.RetainUntil) {
				return lc.ErrStorageContract
			}
			return nil
		}
	}
	if p.LegalHold || p.RetentionMode != "NONE" || p.RetainUntil != nil {
		return lc.ErrStorageContract
	}
	return nil
}

// The source can disappear only while its independently byte-verified ARCHIVED
// sibling remains in a fresh complete inventory with a journaled positive HEAD.
// Evaluate again inside both dispatch and physical-send claim under the fence.
func (d *dependencies) checkRetainedArchive(ctx context.Context, tx pgx.Tx, r lc.DisposalDependencyRequest) error {
	var archive domain.Copy
	for _, c := range d.all {
		if c.Key == "archive/a.csv" && c.Namespace == "dpp-lifecycle307-quarantine" {
			archive = c
		}
	}
	p := d.archivePresence
	i := d.inventory[archive.Namespace]
	if archive.ID == uuid.Nil || archive.DatasetVersionID != r.Copy.DatasetVersionID || archive.ContentSHA256 != r.Copy.ContentSHA256 || archive.ByteSize != r.Copy.ByteSize || p.Copy != archive || p.ObservedAt.After(r.AsOf) || r.AsOf.Sub(p.ObservedAt) > time.Minute || i.ObservedAt.After(r.AsOf) || r.AsOf.Sub(i.ObservedAt) > time.Minute || !inventoryMatches(i, d.all, nil) {
		return lc.ErrStorageContract
	}
	var n int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_copy_operation o JOIN lifecycle_copy_verification v ON v.operation_id=o.id JOIN lifecycle_copy_decision d ON d.operation_id=o.id JOIN lifecycle_storage_call c ON c.id=v.verify_call_id JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE o.workspace_id=$1 AND o.source_copy_id=$2 AND o.target_copy_id=$3 AND o.action='ARCHIVE' AND d.decision='ARCHIVED' AND v.content_sha256=$4 AND v.byte_size=$5 AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL AND x.response_sha256=v.content_sha256 AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=o.target_copy_id)`, workspace, r.Copy.ID, archive.ID, archive.ContentSHA256, archive.ByteSize).Scan(&n)
	if e != nil {
		return e
	}
	if n != 1 {
		return lc.ErrStorageContract
	}
	e = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call c JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE c.workspace_id=$1 AND c.copy_id=$2 AND c.id=$3 AND c.operation='STAT_EXACT' AND x.invoked_at IS NOT NULL AND x.http_status=200 AND x.failure='' AND x.observed_at=$4`, workspace, archive.ID, p.CallID, p.ObservedAt).Scan(&n)
	if e != nil {
		return e
	}
	if n != 1 {
		return lc.ErrStorageContract
	}
	return nil
}
