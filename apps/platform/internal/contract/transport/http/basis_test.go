package contracthttp

import (
	"context"
	"github.com/google/uuid"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"testing"
)

type basisFake struct {
	calls int
	err   error
	query q.BasisQuery
}

func (f *basisFake) ReadBusinessBasis(_ context.Context, query q.BasisQuery) (q.Basis, error) {
	f.calls++
	f.query = query
	return qBasis(), f.err
}
func (f *basisFake) ReadReceiptMaterial(_ context.Context, query q.BasisQuery) (q.MaterialDetail, error) {
	f.calls++
	f.query = query
	return q.MaterialDetail{}, f.err
}
func qBasis() q.Basis { return q.Basis{ProfileVersion: "business-basis.v1"} }
func TestBusinessBasisTransportIsFiniteScopedAndOptIn(t *testing.T) {
	ws := uuid.New()
	actor := uuid.New()
	target := "?deliveryOperationId=" + uuid.NewString() + "&datasetVersionId=" + uuid.NewString()
	path := "/api/v1/workspaces/" + ws.String() + "/acceptance-basis"
	material := "/api/v1/workspaces/" + ws.String() + "/acceptance-materials/" + uuid.NewString()
	resolver, e := principal.NewStaticResolver(true, "synthetic-http-fixture", actor.String(), actor.String(), []string{ws.String()}, []string{principal.CapabilityAcceptanceRead})
	if e != nil {
		t.Fatal(e)
	}
	fake := &basisFake{}
	mux := http.NewServeMux()
	(ReadOnlyHandler{Principal: resolver, Basis: fake, ReadCapability: principal.CapabilityAcceptanceRead}).Register(mux)
	for _, tc := range []struct {
		path   string
		status int
		auth   bool
	}{{path + target, 200, true}, {material + target, 200, true}, {path + target, 401, false}, {path + target + "&actorId=" + actor.String(), 400, true}, {path + target + "&sourceUrl=https://untrusted.invalid", 400, true}, {path + target + "&materialOffset=-1", 400, true}, {path + target + "&materialOffset=2147483648", 400, true}, {path + target + "&datasetVersionId=" + uuid.NewString(), 400, true}, {material + target + "&contractVersionId=" + uuid.NewString(), 400, true}, {path + "?deliveryOperationId=" + uuid.NewString(), 400, true}} {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.auth {
				req.Header.Set("Authorization", "Bearer synthetic-http-fixture")
			}
			before := fake.calls
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("status %d expected %d", rr.Code, tc.status)
			}
			if tc.status != 200 && fake.calls != before {
				t.Fatal("invalid request reached source reader")
			}
			if rr.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("basis cached")
			}
		})
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{q.ErrNotFound, 404}, {principal.ErrWorkspaceDenied, 403}, {principal.ErrAuthorityUnavailable, 503}} {
		fake.err = tc.err
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path+target, nil)
		req.Header.Set("Authorization", "Bearer synthetic-http-fixture")
		mux.ServeHTTP(rr, req)
		if rr.Code != tc.status {
			t.Fatal("read failure became successful/fallback source")
		}
	}
	closed := http.NewServeMux()
	(ReadOnlyHandler{Principal: resolver, Basis: fake}).Register(closed)
	rr := httptest.NewRecorder()
	closed.ServeHTTP(rr, httptest.NewRequest("GET", path+target, nil))
	if rr.Code != 404 {
		t.Fatal("legacy profile gained basis routes")
	}
}
