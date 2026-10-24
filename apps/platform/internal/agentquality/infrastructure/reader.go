package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
)

// Reader reuses the existing repositories and maps only safe context fields.
type Reader struct {
	Catalog     *readmodel.Repository
	DatasetRepo *datasetinfra.PostgresRepository
}

func mapDataset(d readmodel.Dataset) agentquality.Dataset {
	return agentquality.Dataset{ID: d.ID, WorkspaceID: d.WorkspaceID, Code: d.Code, Name: d.Name, DatasetType: d.DatasetType}
}
func notFound(err error) error {
	if errors.Is(err, readmodel.ErrNotFound) || errors.Is(err, datasetinfra.ErrNotFound) {
		return agentquality.ErrNotFound
	}
	return err
}
func (r Reader) Datasets(ctx context.Context, ws uuid.UUID, limit, offset int) (agentquality.Page[agentquality.Dataset], error) {
	p, err := r.Catalog.ListDatasets(ctx, ws, limit, offset)
	result := agentquality.Page[agentquality.Dataset]{Items: []agentquality.Dataset{}, Page: agentquality.PageInfo(p.Page)}
	for _, d := range p.Items {
		result.Items = append(result.Items, mapDataset(d))
	}
	return result, err
}
func (r Reader) Dataset(ctx context.Context, id uuid.UUID) (agentquality.Dataset, error) {
	d, err := r.Catalog.GetDataset(ctx, id)
	return mapDataset(d), notFound(err)
}
func (r Reader) Versions(ctx context.Context, id uuid.UUID, limit, offset int) (agentquality.Page[agentquality.Version], error) {
	p, err := r.Catalog.ListDatasetVersions(ctx, id, limit, offset)
	result := agentquality.Page[agentquality.Version]{Items: []agentquality.Version{}, Page: agentquality.PageInfo(p.Page)}
	for _, v := range p.Items {
		result.Items = append(result.Items, agentquality.Version{ID: v.ID, DatasetID: v.DatasetID, VersionNo: v.VersionNo, Status: v.Status, RowCount: v.RowCount, CreatedAt: v.CreatedAt})
	}
	return result, err
}
func (r Reader) Version(ctx context.Context, id uuid.UUID) (agentquality.Version, error) {
	v, err := r.DatasetRepo.GetVersion(ctx, id)
	return agentquality.Version{ID: v.ID, DatasetID: v.DatasetID, VersionNo: v.VersionNo, Status: string(v.Status), RowCount: v.RowCount, CreatedAt: v.CreatedAt}, notFound(err)
}
