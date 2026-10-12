package main

import (
	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"path/filepath"
)

func selectWorkerDelete() {
	workerDelete = true
	run = "4abcfca5-583d-41ca-8548-10a0fc3270a1"
	workspace = uuid.MustParse("d9deb37f-0e58-4f00-830e-c7aad21895f8")
	planID = uuid.MustParse("872ca588-6856-420f-b450-e5cd4e8008f4")
	planHash = "7184d41a0b1578c09d5f358a31dfce819946154bc6237e259ae5beb680c5c04b"
	inventoryPlanID = uuid.MustParse("3ee654db-c5f6-4f91-811f-dca36f1c5916")
	inventoryHash = "f2bf3ce5dc1b1f2050a75eb271750bd526a64c7f638a9a45bf58fe9f907b27d4"
	providerID = "b48e5dcf-d466-483d-ac89-f810ba976974"
	incarnationID = "57c5084d-d929-4eeb-842d-180734158bb0"
}

// Pinned review manifest; caller locators cannot expand this closed scope.
func workerCopies(c config) ([]domain.Copy, error) {
	var m manifest
	if e := read(filepath.Join(c.OutputDirectory, "manifest.json"), &m); e != nil {
		return nil, e
	}
	expected := map[string]string{
		"dpp-lifecycle307-online/fixture/a.csv":     "4a225512-f6ac-4fc9-b0c0-bd6e016a2a01",
		"dpp-lifecycle307-online/fixture/b.csv":     "7cd23e55-140c-4566-b881-e58c423c7a8a",
		"dpp-lifecycle307-quarantine/fixture/a.csv": "936c8741-87f6-4279-87b9-742936cb2dd2",
		"dpp-lifecycle307-quarantine/fixture/b.csv": "ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2",
	}
	if len(m.Objects) != len(expected) {
		return nil, lc.ErrStorageScope
	}
	var result []domain.Copy
	for _, o := range m.Objects {
		key := o.Namespace + "/" + o.Key
		if expected[key] != o.Version {
			return nil, lc.ErrStorageScope
		}
		delete(expected, key)
		copy := domain.Copy{ID: o.CopyID, WorkspaceID: workspace, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
		if copy.Validate() != nil {
			return nil, lc.ErrStorageScope
		}
		result = append(result, copy)
	}
	if len(expected) != 0 {
		return nil, lc.ErrStorageScope
	}
	return result, nil
}
