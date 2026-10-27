package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsolatedHostAdmitsQueriesAndOnlyPrivateIdentityPost(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/healthz", true}, {"POST", "/internal/identity/business-acceptance-principal", true},
		{"POST", "/api/v1/business-acceptances", false}, {"POST", "/api/agent/tasks", false},
		{"PATCH", "/api/v1/contracts/x", false}, {"DELETE", "/api/v1/workspaces/x", false},
		{"HEAD", "/healthz", false}, {"POST", "/internal/identity/business-acceptance-principal/", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			called := false
			handler := queryOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil))
			if called != tc.allowed {
				t.Fatalf("request admission %t expected %t", called, tc.allowed)
			}
			if !tc.allowed && rr.Code != 405 {
				t.Fatalf("business mutation reached host: %d", rr.Code)
			}
			if rr.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("identity cache allowed")
			}
		})
	}
}
