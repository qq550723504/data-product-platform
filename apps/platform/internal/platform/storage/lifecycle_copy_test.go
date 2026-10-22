package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func copyFixture() domain.CopyOperation {
	c := inspectionFixture()
	payload := []byte("synthetic")
	h := sha256.Sum256(payload)
	c.ContentSHA256 = hex.EncodeToString(h[:])
	c.Key = "fixture/a.csv"
	return domain.CopyOperation{ID: uuid.New(), WorkspaceID: c.WorkspaceID, Source: c, TargetCopyID: uuid.New(), TargetNamespaceID: uuid.New(), TargetNamespace: "lifecycle307-archive", TargetKey: "archive/a.csv", Action: domain.ArchiveCopy, PolicyID: uuid.New(), BasisRef: "synthetic-test", ActorID: uuid.New(), PrincipalRef: "test", DependencyRevision: 1, CreatedAt: time.Now().Add(-time.Second), ExpiresAt: time.Now().Add(time.Minute)}
}
func copierTest(t *testing.T, o domain.CopyOperation, j *inspectionTestJournal, h http.HandlerFunc) *LifecycleCopier {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	s, e := NewLifecycleCopier(LifecycleCopyConfig{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test-access", SecretKey: "test-secret", WorkspaceID: o.WorkspaceID, ProviderInstanceID: o.Source.ProviderInstanceID, IncarnationID: o.Source.IncarnationID, Namespaces: []string{o.Source.Namespace, o.TargetNamespace}, Journal: j})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestNativeCopyExactSourceAndBytesWithoutDelete(t *testing.T) {
	o := copyFixture()
	j := &inspectionTestJournal{}
	var requests atomic.Int64
	s := copierTest(t, o, j, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("X-Amz-Version-Id", "new-version")
		if r.Method == "PUT" {
			if !strings.Contains(r.Header.Get("X-Amz-Copy-Source"), "versionId="+o.Source.ProviderVersion) {
				t.Error("bare source")
			}
			fmt.Fprint(w, `<CopyObjectResult><ETag>"fixture"</ETag><LastModified>2026-10-11T10:00:00Z</LastModified></CopyObjectResult>`)
		} else if r.Method == "GET" && r.URL.Query().Get("versionId") == "new-version" {
			if r.Header.Get("X-Amz-Content-Sha256") != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || !strings.Contains(r.Header.Get("Authorization"), "x-amz-content-sha256") {
				t.Error("exact GET omitted signed request payload hash")
			}
			fmt.Fprint(w, "synthetic")
		} else {
			t.Error("unexpected request")
			w.WriteHeader(403)
		}
	})
	c, id, e := s.CreateCopy(context.Background(), o)
	if e != nil || id == uuid.Nil {
		t.Fatal(e)
	}
	p, e := s.VerifyCopyBytes(context.Background(), o, c)
	if e != nil || p.SHA256 != o.Source.ContentSHA256 || requests.Load() != 2 || len(j.starts) != 2 || len(j.observations) != 2 || j.observations[0].CopyResultVersion != "new-version" {
		t.Fatal("copy/verify proof", e, p)
	}
	wrong := o
	wrong.Source.ProviderVersion = "null"
	if _, _, e = s.CreateCopy(context.Background(), wrong); e == nil || requests.Load() != 2 {
		t.Fatal("invalid source caused I/O")
	}
}
func TestCopyFailureNoImplicitRetryAndHashMismatchNeverVerified(t *testing.T) {
	for _, status := range []int{403, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			o := copyFixture()
			j := &inspectionTestJournal{}
			var calls atomic.Int64
			s := copierTest(t, o, j, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
			})
			if _, _, e := s.CreateCopy(context.Background(), o); e == nil || calls.Load() != 1 || len(j.starts) != 1 || len(j.observations) != 1 {
				t.Fatal("failed copy retried or unjournaled", e, calls.Load())
			}
		})
	}
	o := copyFixture()
	j := &inspectionTestJournal{}
	s := copierTest(t, o, j, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Amz-Version-Id", "bad-version")
		fmt.Fprint(w, "WRONGDATA")
	})
	if _, e := s.VerifyCopyBytes(context.Background(), o, o.Materialized("bad-version")); e == nil || len(j.observations) != 1 || j.observations[0].Failure != "CONTRACT" {
		t.Fatal("corrupt bytes admitted", e)
	}
}
