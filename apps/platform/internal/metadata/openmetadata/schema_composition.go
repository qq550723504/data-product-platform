package openmetadata

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	rights "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

// An absent path deliberately leaves finite discovery disabled. Provider-owned
// launch configuration stays outside the application boundary.
func ConfigureSourceSchema(service *app.Service, pool *pgxpool.Pool, path string) error {
	if path == "" {
		return nil
	}
	observer := &NativeSchemaObserver{Path: path}
	if _, err := observer.Plan(context.Background()); err != nil {
		return err
	}
	service.Schema = &app.SchemaService{Service: service, Observer: observer, Rights: rights.NewPostgresRepository(pool)}
	return nil
}
