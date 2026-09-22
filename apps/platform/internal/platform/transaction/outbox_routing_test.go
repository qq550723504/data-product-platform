package transaction_test

import "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"

func init() {
	router, err := outbox.NewRouter("transaction-test-v1", []outbox.Route{
		{EventType: "TestEventCommitted"},
		{EventType: "TestEventRolledBack"},
	})
	if err != nil {
		panic(err)
	}
	outbox.ConfigureAppendObligation(router)
}
