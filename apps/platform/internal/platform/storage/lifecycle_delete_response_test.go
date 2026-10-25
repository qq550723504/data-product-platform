package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestExactDeleteSafeDiagnosticsAreBoundedAndDoNotExposeProviderText(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		complete         bool
	}{
		{"native", `<Error><Code>AccessDenied</Code><Message>secret-signature</Message><Resource>secret-signature</Resource></Error>`, "AccessDenied", true},
		{"signature", `<Error><Code>SignatureDoesNotMatch</Code><CanonicalRequest>secret-signature</CanonicalRequest></Error>`, "SignatureDoesNotMatch", true},
		{"reflected-code", `<Error><Code>secret-signature</Code></Error>`, "UNRECOGNIZED_ERROR_CODE", true},
		{"duplicate-code", `<Error><Code>AccessDenied</Code><Code>secret-signature</Code></Error>`, "UNPARSEABLE_ERROR", true},
		{"wrong-root", `<Other><Code>AccessDenied</Code></Other>`, "UNPARSEABLE_ERROR", true},
		{"trailing", `<Error><Code>AccessDenied</Code></Error><Other/>`, "UNPARSEABLE_ERROR", true},
		{"truncated", `<Error><Code>AccessDenied</Code></Error>`, "INCOMPLETE_ERROR", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := InspectExactDeleteResponse(403, []byte(tc.body), tc.complete)
			raw, _ := json.Marshal(r)
			if r.ErrorCode != tc.code || len(r.BodySHA256) != 64 || strings.Contains(string(raw), "secret-signature") {
				t.Fatal(string(raw))
			}
		})
	}
}

func TestExactDelete403RetainsReasonAndOriginalObservationWhenJournalFails(t *testing.T) {
	for _, failed := range []bool{false, true} {
		c, a := deleteFixture()
		j := &deleteJournal{failObservation: failed}
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(403)
			w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>secret-signature</Message></Error>`))
		}))
		defer s.Close()
		d, e := NewLifecycleDeleter(LifecycleDeleteConfig{Endpoint: strings.TrimPrefix(s.URL, "http://"), AccessKey: "test", SecretKey: "secret", Targets: []domain.Copy{c}, Journal: j})
		if e != nil {
			t.Fatal(e)
		}
		err := d.DeleteExactCopy(context.Background(), a, c)
		var f *ExactDeleteFailure
		if !errors.As(err, &f) || f.Response.ErrorCode != "AccessDenied" || f.Response.HTTPStatus != 403 || strings.Contains(err.Error(), "secret-signature") || calls != 1 {
			t.Fatal(err, calls)
		}
		want := lc.ErrStorageDenied
		if failed {
			want = lc.ErrStorageUnknown
		}
		if !errors.Is(err, want) {
			t.Fatal(err)
		}
		if j.proof.Outcome != "DENIED" || j.proof.InvokedAt == nil {
			t.Fatal(j.proof)
		}
		if d.DeleteExactCopy(context.Background(), a, c) == nil || calls != 1 {
			t.Fatal("resend")
		}
	}
}
