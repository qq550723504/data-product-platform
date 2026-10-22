package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/sourceadmission"
	"net/http"
	"os"
)

func sourceAdmissionRoutes(pool *pgxpool.Pool) (func(*http.ServeMux), error) {
	path := os.Getenv("FORK_APPROVED_SOURCE_PROFILE")
	if path == "" {
		return func(*http.ServeMux) {}, nil
	}
	h, e := sourceadmission.Build(pool, path)
	if e != nil {
		return nil, e
	}
	return h.Register, nil
}
