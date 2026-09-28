package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewProductReleaseRequiresExactlyOneProductionTarget(t *testing.T) {
	productID := uuid.New()
	versionID := uuid.New()

	cases := []struct {
		name     string
		datasets []ReleaseDataset
	}{
		{
			name: "no production target",
			datasets: []ReleaseDataset{
				{DatasetVersionID: uuid.New(), Role: DatasetInput},
			},
		},
		{
			name: "multiple production targets",
			datasets: []ReleaseDataset{
				{DatasetVersionID: uuid.New(), Role: DatasetPrimary},
				{DatasetVersionID: uuid.New(), Role: DatasetOutput},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewProductRelease(productID, versionID, "R-1", tc.datasets, "", nil, nil); err == nil {
				t.Fatal("expected invalid ProductRelease production-target membership")
			}
		})
	}

	if _, err := NewProductRelease(productID, versionID, "R-2", []ReleaseDataset{
		{DatasetVersionID: uuid.New(), Role: DatasetPrimary},
		{DatasetVersionID: uuid.New(), Role: DatasetSupporting},
	}, "", nil, nil); err != nil {
		t.Fatalf("single production target should be valid: %v", err)
	}
}
