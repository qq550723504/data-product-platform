package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

type custodyJournalTest struct {
	starts                 int
	observations           []lc.CustodyObservation
	failStart, failObserve bool
}

func (j *custodyJournalTest) StartCustodyCall(_ context.Context, _ int) (uuid.UUID, error) {
	j.starts++
	if j.failStart {
		return uuid.Nil, errors.New("private start failure")
	}
	return uuid.New(), nil
}
func (j *custodyJournalTest) ObserveCustodyCall(ctx context.Context, o lc.CustodyObservation) error {
	if ctx.Err() != nil {
		return errors.New("cancelled outcome")
	}
	if j.failObserve {
		return errors.New("private journal failure")
	}
	j.observations = append(j.observations, o)
	return nil
}
func custodyTestPlan(endpoint string, b []byte) lc.CustodyPlan {
	h := sha256.Sum256(b)
	hash := hex.EncodeToString(h[:])
	return lc.CustodyPlan{WorkspaceID: uuid.New(), ActorID: uuid.New(), RunID: "synthetic", PrincipalRef: "private-test-operator", ProviderWitness: "synthetic-provider", Endpoint: endpoint, OutputDirectory: "test-private-not-created", PlanSHA256: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(time.Minute),
		First: lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/first.csv", SHA256: hash, Bytes: int64(len(b))}, Second: lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/second.csv", SHA256: hash, Bytes: int64(len(b))}}
}
func TestCustodySDKOneExactGETFailureAccountingAndNoSecretErrors(t *testing.T) {
	b := []byte("id,value\n1,unchanged\n")
	for _, mode := range []string{"ok", "wrong-hash", "oversized", "truncated", "503", "redirect", "start-denied", "observation-denied"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/raw/datasets/first.csv" || r.URL.RawQuery != "" || r.Header.Get("Range") != "" || r.Header.Get("Authorization") == "" {
					t.Errorf("unexpected SDK request %s %s", r.Method, r.URL.Path)
				}
				if mode == "503" {
					w.WriteHeader(503)
					_, _ = io.WriteString(w, "<Error><Code>SlowDown</Code><Message>private-provider-secret</Message></Error>")
					return
				}
				if mode == "redirect" {
					w.Header().Set("Location", "http://127.0.0.1:1/stolen")
					w.WriteHeader(307)
					return
				}
				data := append([]byte{}, b...)
				if mode == "wrong-hash" {
					data[0] = 'x'
				}
				if mode == "oversized" {
					data = append(data, 'x')
				}
				length := len(data)
				if mode == "truncated" {
					length += 7
				}
				w.Header().Set("Content-Length", fmt.Sprint(length))
				w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				w.Header().Set("ETag", `"not-content-proof"`)
				_, _ = w.Write(data)
			}))
			defer server.Close()
			p := custodyTestPlan(strings.TrimPrefix(server.URL, "http://"), b)
			a, err := NewOperatorCustodyAdapter(p, credentials.NewStaticV4("private-access", "private-secret", ""))
			if err != nil {
				t.Fatal(err)
			}
			j := &custodyJournalTest{failStart: mode == "start-denied", failObserve: mode == "observation-denied"}
			got, err := a.ReadCustody(context.Background(), 1, p.First, j)
			if mode == "ok" {
				if err != nil || string(got) != string(b) {
					t.Fatalf("body proof: %v", err)
				}
			} else if err == nil || got != nil {
				t.Fatal("failed invocation published content")
			}
			if err != nil && strings.Contains(err.Error(), "private-") {
				t.Fatal("provider/credential error leaked")
			}
			want := 1
			if mode == "start-denied" {
				want = 0
			}
			if calls != want || j.starts != 1 {
				t.Fatalf("wire=%d starts=%d", calls, j.starts)
			}
			if mode == "start-denied" || mode == "observation-denied" {
				if len(j.observations) != 0 {
					t.Fatal("invented durable outcome")
				}
				return
			}
			if len(j.observations) != 1 || j.observations[0].HTTPStatus == 0 || j.observations[0].InvocationCertainty != "CONFIRMED" {
				t.Fatal("failed/redirect call lost observation")
			}
			if (j.observations[0].Failure == "") != (mode == "ok") {
				t.Fatal("wrong failure proof")
			}
		})
	}
}

type custodyRoundTripFunc func(*http.Request) (*http.Response, error)

func (f custodyRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCustodyWireRejectsAllAlternateCallsBeforeJournalOrTransport(t *testing.T) {
	for _, mutation := range []func(*http.Request){
		func(r *http.Request) { r.Method = "HEAD" }, func(r *http.Request) { r.Method = "PUT" }, func(r *http.Request) { r.Method = "DELETE" },
		func(r *http.Request) { r.URL.Host = "127.0.0.1:2" }, func(r *http.Request) { r.URL.RawQuery = "versionId=guessed" },
		func(r *http.Request) { r.URL.Path = "/raw/other" }, func(r *http.Request) { r.Header.Set("Range", "bytes=0-1") },
		func(r *http.Request) { r.URL.Path = "/raw" }, func(r *http.Request) { r.URL.Path = "/minio/health/live" },
	} {
		j := &custodyJournalTest{}
		forwarded := 0
		w := &custodyWire{endpoint: "127.0.0.1:1", path: "/raw/datasets/first.csv", member: 1, journal: j, base: custodyRoundTripFunc(func(*http.Request) (*http.Response, error) { forwarded++; return nil, errors.New("should not forward") })}
		r, _ := http.NewRequest("GET", "http://127.0.0.1:1/raw/datasets/first.csv", nil)
		mutation(r)
		if _, err := w.RoundTrip(r); err == nil || j.starts != 0 || forwarded != 0 {
			t.Fatal("alternate request crossed boundary")
		}
	}
}

func TestCustodyTransportUnknownStillRecordsAttempt(t *testing.T) {
	j := &custodyJournalTest{}
	w := &custodyWire{endpoint: "127.0.0.1:1", path: "/raw/datasets/first.csv", member: 1, journal: j, base: custodyRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private transport error") })}
	r, _ := http.NewRequest("GET", "http://127.0.0.1:1/raw/datasets/first.csv", nil)
	if _, err := w.RoundTrip(r); err == nil || !w.dispatched || w.observation.InvocationCertainty != "UNKNOWN" || w.observation.CallID == uuid.Nil {
		t.Fatal("transport uncertainty missing physical start")
	}
	if _, err := w.RoundTrip(r); err == nil || j.starts != 1 {
		t.Fatal("retry crossed finite dispatch budget")
	}
}

type custodyCloseFailure struct{ io.ReadCloser }

func (b custodyCloseFailure) Close() error {
	return errors.Join(b.ReadCloser.Close(), errors.New("private close failure"))
}

func TestCustodySynchronousSDKPropagatesCloseFailureAndJournalsUnknown(t *testing.T) {
	b := []byte("id,value\n1,unchanged\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		_, _ = w.Write(b)
	}))
	defer server.Close()
	p := custodyTestPlan(strings.TrimPrefix(server.URL, "http://"), b)
	a, _ := NewOperatorCustodyAdapter(p, credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""))
	base := &http.Transport{DisableKeepAlives: true}
	defer base.CloseIdleConnections()
	a.transport = custodyRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(r)
		if resp != nil {
			resp.Body = custodyCloseFailure{resp.Body}
		}
		return resp, err
	})
	j := &custodyJournalTest{}
	if got, err := a.ReadCustody(context.Background(), 1, p.First, j); err == nil || got != nil {
		t.Fatal("SDK Close error published content")
	}
	if len(j.observations) != 1 || j.observations[0].CloseOK || j.observations[0].Failure == "" {
		t.Fatal("Close failure not durable")
	}
	a.transport = custodyRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private transport failure") })
	j = &custodyJournalTest{}
	if got, err := a.ReadCustody(context.Background(), 1, p.First, j); err == nil || got != nil {
		t.Fatal("UNKNOWN transport published content")
	}
	if len(j.observations) != 1 || j.observations[0].InvocationCertainty != "UNKNOWN" || j.observations[0].Failure != "TRANSPORT" {
		t.Fatal("UNKNOWN invocation lost outcome/cost input")
	}
}
