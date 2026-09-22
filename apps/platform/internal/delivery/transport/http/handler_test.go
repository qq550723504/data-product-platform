package deliveryhttp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type fakeDirectService struct {
	result deliveryapp.DirectDataResult
	err    error
	calls  int
	order  *[]string
}

func (s *fakeDirectService) Deliver(_ context.Context, _ deliveryapp.DirectDataCommand) (deliveryapp.DirectDataResult, error) {
	s.calls++
	if s.order != nil {
		*s.order = append(*s.order, "service")
	}
	return s.result, s.err
}

type fakeObjectStore struct {
	content []byte
	calls   int
	order   *[]string
}

func (s *fakeObjectStore) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	s.calls++
	if s.order != nil {
		*s.order = append(*s.order, "store")
	}
	return io.NopCloser(bytes.NewReader(s.content)), nil
}

func TestDeliveryHandlerRejectsConsumerSpoofBeforeCommand(t *testing.T) {
	workspaceID, versionID, profileID := uuid.New(), uuid.New(), uuid.New()
	resolver, err := NewStaticPrincipalResolver(true, "secret", "principal-a", "consumer-a", []string{workspaceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeDirectService{}
	store := &fakeObjectStore{}
	response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-b", "key-spoof", "secret")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "CONSUMER_PRINCIPAL_MISMATCH") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.calls != 0 || store.calls != 0 {
		t.Fatalf("spoofed consumer reached command/storage: service=%d store=%d", service.calls, store.calls)
	}
}

func TestDeliveryHandlerBlockedAndReplayReturnZeroPayload(t *testing.T) {
	workspaceID, versionID, profileID := uuid.New(), uuid.New(), uuid.New()
	resolver, err := NewStaticPrincipalResolver(true, "secret", "principal-a", "consumer-a", []string{workspaceID.String()})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("blocked", func(t *testing.T) {
		service := &fakeDirectService{result: deliveryapp.DirectDataResult{
			Operation: deliverydomain.Operation{ID: uuid.New(), Status: deliverydomain.StatusBlocked},
			Blockers:  []string{"DATASET_VERSION_INVALID"},
		}}
		store := &fakeObjectStore{content: []byte("must-not-leak")}
		response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-a", "key-blocked", "secret")
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "DATASET_VERSION_INVALID") {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		if store.calls != 0 || strings.Contains(response.Body.String(), "must-not-leak") {
			t.Fatalf("blocked command leaked payload: calls=%d body=%s", store.calls, response.Body.String())
		}
	})

	t.Run("same-key issued replay", func(t *testing.T) {
		service := &fakeDirectService{
			result: deliveryapp.DirectDataResult{Operation: deliverydomain.Operation{ID: uuid.New(), Status: deliverydomain.StatusIssued}, ReplayRequired: true},
			err:    deliveryapp.ErrDirectDataReplayRequiresNewAttempt,
		}
		store := &fakeObjectStore{content: []byte("must-not-replay")}
		response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-a", "key-replay", "secret")
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT") {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		if store.calls != 0 || strings.Contains(response.Body.String(), "must-not-replay") {
			t.Fatalf("replay leaked payload: calls=%d body=%s", store.calls, response.Body.String())
		}
	})
}

func TestDeliveryHandlerMapsTransactionalRightsLookupMissToNotFound(t *testing.T) {
	workspaceID, versionID, profileID := uuid.New(), uuid.New(), uuid.New()
	resolver, err := NewStaticPrincipalResolver(true, "secret", "principal-a", "consumer-a", []string{workspaceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeDirectService{err: rightsinfra.ErrNotFound}
	store := &fakeObjectStore{content: []byte("must-not-read")}
	response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-a", "key-missing", "secret")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "DELIVERY_TARGET_NOT_FOUND") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if store.calls != 0 {
		t.Fatalf("not-found delivery opened object storage %d times", store.calls)
	}
}

func TestDeliveryHandlerReadsObjectOnlyAfterIssuedCommandReturns(t *testing.T) {
	workspaceID, versionID, profileID := uuid.New(), uuid.New(), uuid.New()
	resolver, err := NewStaticPrincipalResolver(true, "secret", "principal-a", "consumer-a", []string{workspaceID.String()})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	payload := []byte("certified-dataset-bytes")
	operationID := uuid.New()
	size := int64(len(payload))
	service := &fakeDirectService{
		result: deliveryapp.DirectDataResult{
			Operation: deliverydomain.Operation{ID: operationID, Status: deliverydomain.StatusIssued},
			DatasetVersion: datasetdomain.DatasetVersion{
				ID: versionID, StorageURI: "s3://bucket/object", ContentType: "text/csv", ByteSize: &size,
			},
			PayloadReady: true,
		},
		order: &order,
	}
	store := &fakeObjectStore{content: payload, order: &order}
	response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-a", "key-issued", "secret")
	if response.Code != http.StatusOK || response.Body.String() != string(payload) {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Delivery-Operation-Id") != operationID.String() {
		t.Fatalf("operation header = %q", response.Header().Get("X-Delivery-Operation-Id"))
	}
	if len(order) != 2 || order[0] != "service" || order[1] != "store" {
		t.Fatalf("call order = %v, want [service store]", order)
	}
}

func TestDeliveryHandlerFailsClosedWhenResolverIsDisabled(t *testing.T) {
	workspaceID, versionID, profileID := uuid.New(), uuid.New(), uuid.New()
	resolver, err := NewStaticPrincipalResolver(false, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeDirectService{}
	store := &fakeObjectStore{}
	response := executeDeliveryRequest(t, NewHandler(service, resolver, store), workspaceID, versionID, profileID, "consumer-a", "key-disabled", "anything")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "DIRECT_DATA_DELIVERY_NOT_CONFIGURED") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.calls != 0 || store.calls != 0 {
		t.Fatalf("disabled delivery reached command/storage: service=%d store=%d", service.calls, store.calls)
	}
}

func executeDeliveryRequest(t *testing.T, handler *Handler, workspaceID, versionID, profileID uuid.UUID, consumer, key, token string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"profileId":"` + profileID.String() + `","consumer":"` + consumer + `","purpose":"RESEARCH","action":"READ","scopeType":"ALL_RESOURCE"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspaceID.String()+"/dataset-versions/"+versionID.String()+"/deliveries", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	handler.Register(mux)
	mux.ServeHTTP(response, request)
	return response
}
