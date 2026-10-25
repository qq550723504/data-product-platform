package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestWireRequiresActualUpstreamResponseBeforeContainmentAndNeverResends(t *testing.T) {
	for _, caseName := range []string{"H2", "M1B", "P2"} {
		t.Run(caseName, func(t *testing.T) {
			group := "business-races"
			if caseName == "H2" {
				group = "late-hold"
			}
			if caseName == "M1B" {
				group = "mixed"
			}
			cp, e := fixedTarget(group, caseName)
			if e != nil {
				t.Fatal(e)
			}
			public, key, _ := ed25519.GenerateKey(rand.Reader)
			var accepted atomic.Int32
			var contained atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" {
					t.Error("wrong method")
				}
				accepted.Add(1)
				w.WriteHeader(204)
			}))
			defer upstream.Close()
			backend, _ := url.Parse(upstream.URL)
			budget, out := t.TempDir(), t.TempDir()
			h := &wireHandler{scope: []domain.Copy{cp}, access: "fixed-delete", key: key, backend: backend, transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, output: out, budget: budget, fresh: func() error { return nil }, attempt: func(context.Context, domain.Copy) (domain.ProviderAttempt, error) {
				return domain.ProviderAttempt{ID: uuid.New(), CopyID: cp.ID, WorkspaceID: cp.WorkspaceID}, nil
			}}
			h.after = func(_ context.Context, p signed) error {
				if accepted.Load() != 1 || !ed25519.Verify(public, p.Body, p.Signature) {
					t.Error("containment preceded actual native response or witness not signed")
				}
				var w wireWitness
				if json.Unmarshal(p.Body, &w) != nil || w.UpstreamStatus != 204 || w.Copy != cp {
					t.Error("bad witness")
				}
				contained.Store(true)
				return nil
			}
			proxy := httptest.NewServer(h)
			defer proxy.Close()
			client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
			request := func(method, query string) (int, error) {
				req, _ := http.NewRequest(method, proxy.URL+"/"+cp.Namespace+"/"+cp.Key+query, nil)
				req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=fixed-delete/example")
				response, e := client.Do(req)
				if e != nil {
					return 0, e
				}
				defer response.Body.Close()
				io.Copy(io.Discard, response.Body)
				return response.StatusCode, nil
			}
			if status, _ := request("DELETE", "?versionId=foreign"); status != 403 {
				t.Fatal("foreign version forwarded")
			}
			status, e := request("DELETE", "?versionId="+cp.ProviderVersion)
			if caseName == "P2" {
				if e != nil || status != 204 {
					t.Fatal(status, e)
				}
			} else if e == nil {
				t.Fatal("response loss not exercised")
			}
			if accepted.Load() != 1 || contained.Load() != (caseName == "H2") {
				t.Fatal("wire/containment ordering differs")
			}
			if status, _ = request("DELETE", "?versionId="+cp.ProviderVersion); status != 403 || accepted.Load() != 1 {
				t.Fatal("second send forwarded")
			}
			var proof signed
			if read(filepath.Join(out, "wire-"+cp.ID.String()+".json"), &proof) != nil || !ed25519.Verify(public, proof.Body, proof.Signature) {
				t.Fatal("durable witness missing")
			}
		})
	}
}
func TestCompiledRemainingTargetsExcludeSurvivorsAndOldProfiles(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		for _, stage := range []string{"H0", "H1", "M1C", "CONTROL", "send-a", "send-b", "9ea8f3e2-c805-44b4-b133-a36f50df9d15"} {
			if _, e := fixedTarget(group, stage); e == nil {
				t.Fatal("unconfirmed target accepted", stage)
			}
		}
	}
	if _, e := fixedTarget("5d2274ee", "P2"); e == nil {
		t.Fatal("old environment accepted")
	}
}

func TestWireSignsNativeDenialSeparatelyFromLocalRefusalWithoutSecrets(t *testing.T) {
	cp, _ := fixedTarget("business-races", "P2")
	public, key, _ := ed25519.GenerateKey(rand.Reader)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(403)
		w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>secret-signature</Message></Error>`))
	}))
	defer upstream.Close()
	backend, _ := url.Parse(upstream.URL)
	out, budget := t.TempDir(), t.TempDir()
	h := &wireHandler{scope: []domain.Copy{cp}, access: "test-delete", key: key, backend: backend, transport: http.DefaultTransport, output: out, budget: budget, fresh: func() error { return nil }, attempt: func(context.Context, domain.Copy) (domain.ProviderAttempt, error) {
		return domain.ProviderAttempt{ID: uuid.New(), CopyID: cp.ID}, nil
	}}
	s := httptest.NewServer(h)
	defer s.Close()
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("DELETE", s.URL+"/"+cp.Namespace+"/"+cp.Key+"?versionId="+cp.ProviderVersion, nil)
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test-delete/example")
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 403 {
			t.Fatal(r.StatusCode)
		}
	}
	if calls != 1 {
		t.Fatal("wire resent native denial")
	}
	files, _ := os.ReadDir(out)
	if len(files) != 2 {
		t.Fatal("missing native or local rejection")
	}
	for _, f := range files {
		var s signed
		if read(filepath.Join(out, f.Name()), &s) != nil || !ed25519.Verify(public, s.Body, s.Signature) || strings.Contains(string(s.Body), "secret-signature") {
			t.Fatal("unsigned or unsafe diagnostics")
		}
	}
}
