package migration_test

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func init() {
	router, err := routing.NewRouter(false)
	if err != nil {
		panic(err)
	}
	outbox.ConfigureAppendObligation(router)
}
