package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

type protectedTestJournal struct {
	mu           sync.Mutex
	methods      map[uuid.UUID]string
	observations []lc.StorageCallObservation
	failGET      bool
	failPUT      bool
}

func (j *protectedTestJournal) StartProtectedCall(_ context.Context, _ lc.ProtectedObjectIntent, m string) (uuid.UUID, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	id := uuid.New()
	j.methods[id] = m
	return id, nil
}
func (j *protectedTestJournal) ObserveProtectedCall(ctx context.Context, o lc.StorageCallObservation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if ctx.Err() != nil {
		return errors.New("observation cancelled with provider request")
	}
	if (j.failGET && j.methods[o.CallID] == "GET") || (j.failPUT && j.methods[o.CallID] == "PUT") {
		return errors.New("durable journal unavailable")
	}
	j.observations = append(j.observations, o)
	return nil
}

func TestProtectedSDKReadPersistsEOFProofAndPropagatesJournalFailure(t *testing.T) {
	data := []byte("id,value\n1,unchanged\n")
	hash := sha256.Sum256(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Length", "0")
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", `"controlled"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method == "GET" {
			_, _ = w.Write(data)
		}
	}))
	defer server.Close()
	for _, fail := range []bool{false, true} {
		j := &protectedTestJournal{methods: map[uuid.UUID]string{}, failGET: fail}
		p, err := NewProtectedAdapter(strings.TrimPrefix(server.URL, "http://"), "raw", false, credentials.NewStaticV4("controlled-access", "controlled-secret", "controlled-session"), j)
		if err != nil {
			t.Fatal(err)
		}
		i := lc.ProtectedObjectIntent{ID: uuid.New(), WorkspaceID: uuid.New(), NamespaceID: uuid.New(), DatasetVersionID: uuid.New(), Key: "core/controlled/file.csv", SHA256: hex.EncodeToString(hash[:]), ByteSize: int64(len(data)), ContentType: "text/csv", Purpose: "CREATE"}
		got, err := p.ReadProtected(context.Background(), i)
		if fail {
			if !errors.Is(err, lc.ErrProtectedJournal) || got != nil {
				t.Fatalf("journal failure exposed bytes: %v", err)
			}
			continue
		}
		if err != nil || string(got) != string(data) {
			t.Fatalf("SDK read: %v", err)
		}
		j.mu.Lock()
		found := false
		for _, o := range j.observations {
			found = found || (j.methods[o.CallID] == "GET" && o.ResponseSHA256 == i.SHA256 && o.Failure == "")
		}
		j.mu.Unlock()
		if !found {
			t.Fatal("SDK returned before durable EOF/body proof")
		}
	}
	j := &protectedTestJournal{methods: map[uuid.UUID]string{}, failPUT: true}
	p, err := NewProtectedAdapter(strings.TrimPrefix(server.URL, "http://"), "raw", false, credentials.NewStaticV4("controlled-access", "controlled-secret", "controlled-session"), j)
	if err != nil {
		t.Fatal(err)
	}
	i := lc.ProtectedObjectIntent{ID: uuid.New(), Key: "core/controlled/file.csv", SHA256: hex.EncodeToString(hash[:]), ByteSize: int64(len(data)), ContentType: "text/csv"}
	if err = p.CreateProtected(context.Background(), i, data); !errors.Is(err, lc.ErrProtectedJournal) {
		t.Fatalf("SDK swallowed PUT journal close error: %v", err)
	}
}

type protectedFailedTransport struct{}

func (protectedFailedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("controlled connection reset")
}

func TestProtectedTransportFailureCannotHideObservationFailure(t *testing.T) {
	j := &protectedTestJournal{methods: map[uuid.UUID]string{}, failPUT: true}
	transport := &protectedTransport{base: protectedFailedTransport{}, endpoint: "controlled", bucket: "raw", journal: j}
	i := lc.ProtectedObjectIntent{ID: uuid.New(), Key: "core/controlled/file.csv"}
	ctx := context.WithValue(context.Background(), protectedIntentKey{}, i)
	req, err := http.NewRequestWithContext(ctx, "PUT", "http://controlled/raw/"+i.Key, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-None-Match", "*")
	_, err = transport.RoundTrip(req)
	if !errors.Is(err, lc.ErrProtectedJournal) {
		t.Fatalf("uncertain invocation may reconcile without durable observation: %v", err)
	}
}
