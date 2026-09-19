package acceptance_test

import (
	"os"
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func TestMain(m *testing.M) {
	router, err := routing.NewRouter(false)
	if err != nil {
		panic(err)
	}
	outbox.ConfigureAppendObligation(router)
	code := m.Run()
	outbox.ConfigureAppendObligation(nil)
	os.Exit(code)
}
