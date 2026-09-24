package annotationhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type fakeReviewService struct {
	calls  int
	last   annotationapp.ReviewAnnotationCommand
	result annotationapp.ReviewAnnotationResult
	err    error
}

func (f *fakeReviewService) ReviewAnnotation(_ context.Context, cmd annotationapp.ReviewAnnotationCommand) (annotationapp.ReviewAnnotationResult, error) {
	f.calls++
	f.last = cmd
	return f.result, f.err
}

func TestAnnotationReviewHTTPUsesTrustedPrincipalActor(t *testing.T) {
	workspaceID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	trustedActor := uuid.New()
	forgedActor := uuid.New()

	resolver, err := platformprincipal.NewStaticResolver(
		true,
		"review-secret",
		"reviewer:trusted",
		trustedActor.String(),
		[]string{workspaceID.String()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}
	attemptID := uuid.New()
	decisionID := uuid.New()
	service := &fakeReviewService{
		result: annotationapp.ReviewAnnotationResult{
			Attempt: annotationdomain.ReviewAttempt{
				ID: attemptID, WorkspaceID: workspaceID, CampaignID: campaignID, TaskID: taskID,
				ReviewerRef: trustedActor.String(), ExpectedTaskRevision: 2, Action: annotationdomain.ReviewAccept,
				Reason: "verified", IdempotencyKey: "review-1", RequestFingerprint: strings.Repeat("a", 64), CreatedAt: time.Now().UTC(),
			},
			Outcome: annotationdomain.ReviewAttemptOutcome{
				ID: uuid.New(), AttemptID: attemptID, Outcome: annotationdomain.ReviewAttemptSucceeded, OccurredAt: time.Now().UTC(),
			},
			Decision: &annotationdomain.ReviewDecision{
				ID: decisionID, WorkspaceID: workspaceID, CampaignID: campaignID, TaskID: taskID,
				ReviewAttemptID: attemptID, ReviewedResultID: &resultID, SelectedResultID: &resultID,
				ReviewerRef: trustedActor.String(), Outcome: annotationdomain.ReviewAccept, Reason: "verified",
				ExpectedTaskRevision: 2, CreatedAt: time.Now().UTC(),
			},
		},
	}

	mux := http.NewServeMux()
	NewHandler(service, resolver).Register(mux)
	body := `{"expectedTaskRevision":2,"action":"ACCEPT","reason":" verified ","reviewedResultId":"` + resultID.String() + `"}`
	request := httptest.NewRequest(http.MethodPost, reviewPath(workspaceID, campaignID, taskID), strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer review-secret")
	request.Header.Set("Idempotency-Key", "review-1")
	request.Header.Set("X-Actor-ID", forgedActor.String())
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want 1", service.calls)
	}
	if service.last.ActorID == nil || *service.last.ActorID != trustedActor {
		t.Fatalf("actor = %v, want trusted %s", service.last.ActorID, trustedActor)
	}
	if service.last.ReviewerRef != trustedActor.String() {
		t.Fatalf("reviewerRef = %q, want trusted actor", service.last.ReviewerRef)
	}
	if service.last.ActorID != nil && *service.last.ActorID == forgedActor {
		t.Fatal("forged X-Actor-ID became authoritative")
	}
	if service.last.Reason != "verified" || service.last.IdempotencyKey != "review-1" {
		t.Fatalf("normalized command = %#v", service.last)
	}
}

func TestAnnotationReviewHTTPAuthAndFailClosedErrors(t *testing.T) {
	workspaceID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	actorID := uuid.New()

	resolver, err := platformprincipal.NewStaticResolver(
		true,
		"review-secret",
		"reviewer:trusted",
		actorID.String(),
		[]string{workspaceID.String()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}
	validBody := `{"expectedTaskRevision":2,"action":"ACCEPT","reason":"verified","reviewedResultId":"` + resultID.String() + `"}`

	t.Run("unauthenticated", func(t *testing.T) {
		service := &fakeReviewService{}
		recorder := performReview(t, service, resolver, workspaceID, campaignID, taskID, "", "review-1", validBody)
		if recorder.Code != http.StatusUnauthorized || service.calls != 0 {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
	})

	t.Run("foreign workspace", func(t *testing.T) {
		service := &fakeReviewService{}
		recorder := performReview(t, service, resolver, uuid.New(), campaignID, taskID, "review-secret", "review-1", validBody)
		if recorder.Code != http.StatusForbidden || service.calls != 0 {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
	})

	t.Run("missing reason", func(t *testing.T) {
		service := &fakeReviewService{}
		body := `{"expectedTaskRevision":2,"action":"ACCEPT","reason":" ","reviewedResultId":"` + resultID.String() + `"}`
		recorder := performReview(t, service, resolver, workspaceID, campaignID, taskID, "review-secret", "review-1", body)
		if recorder.Code != http.StatusBadRequest || service.calls != 0 {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
	})

	t.Run("stale revision", func(t *testing.T) {
		service := &fakeReviewService{err: annotationinfra.ErrStaleRevision}
		recorder := performReview(t, service, resolver, workspaceID, campaignID, taskID, "review-secret", "review-1", validBody)
		if recorder.Code != http.StatusConflict || service.calls != 1 || !strings.Contains(recorder.Body.String(), "ANNOTATION_REVIEW_STALE") {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
	})

	t.Run("idempotency conflict", func(t *testing.T) {
		service := &fakeReviewService{err: annotationapp.ErrIdempotencyConflict}
		recorder := performReview(t, service, resolver, workspaceID, campaignID, taskID, "review-secret", "review-1", validBody)
		if recorder.Code != http.StatusConflict || service.calls != 1 || !strings.Contains(recorder.Body.String(), "ANNOTATION_REVIEW_IDEMPOTENCY_CONFLICT") {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
	})

	t.Run("generic domain failure remains closed", func(t *testing.T) {
		service := &fakeReviewService{err: errors.New("database details must not leak")}
		recorder := performReview(t, service, resolver, workspaceID, campaignID, taskID, "review-secret", "review-1", validBody)
		if recorder.Code != http.StatusConflict || service.calls != 1 {
			t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "database details") {
			t.Fatalf("internal error leaked: %s", recorder.Body.String())
		}
	})
}

func performReview(
	t *testing.T,
	service ReviewService,
	resolver platformprincipal.Resolver,
	workspaceID, campaignID, taskID uuid.UUID,
	token, idempotencyKey, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(service, resolver).Register(mux)
	request := httptest.NewRequest(http.MethodPost, reviewPath(workspaceID, campaignID, taskID), strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func reviewPath(workspaceID, campaignID, taskID uuid.UUID) string {
	return "/api/v1/workspaces/" + workspaceID.String() +
		"/annotation-campaigns/" + campaignID.String() +
		"/tasks/" + taskID.String() + "/review"
}
