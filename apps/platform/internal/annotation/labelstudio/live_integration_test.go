package labelstudio_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/labelstudio"
)

func TestLabelStudioLiveReference(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_URL"))
	token := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_TOKEN"))
	if baseURL == "" || token == "" {
		t.Skip("TEST_LABEL_STUDIO_URL and TEST_LABEL_STUDIO_TOKEN are required")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	client, err := labelstudio.NewClient(baseURL, token, "ci-label-studio", httpClient)
	if err != nil {
		t.Fatalf("new Label Studio client: %v", err)
	}
	accessToken := refreshPersonalAccessToken(t, httpClient, baseURL, token)

	workspaceID := uuid.New()
	campaignID := uuid.New()
	requestID := "campaign-" + uuid.NewString()
	schema := `{"kind":"single-label-v1","labels":["EVIDENCE_SUFFICIENT","EVIDENCE_INSUFFICIENT","EVIDENCE_CONFLICT"]}`
	schemaHash := sha256HexString(schema)
	projectRequest := annotationapp.EngineCampaignRequest{
		WorkspaceID:        workspaceID,
		CampaignID:         campaignID,
		RequestID:          requestID,
		RequestFingerprint: strings.Repeat("a", 64),
		Title:              "gold-ci-" + campaignID.String()[:8],
		SchemaContent:      schema,
		SchemaSHA256:       schemaHash,
	}

	binding, err := client.EnsureCampaignBinding(context.Background(), projectRequest)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	defer deleteProject(t, httpClient, baseURL, accessToken, binding.ExternalProjectID)

	lookup, err := client.LookupCampaignBinding(context.Background(), projectRequest)
	if err != nil {
		t.Fatalf("lookup project: %v", err)
	}
	if lookup.State != annotationapp.EngineLookupMatched || lookup.Binding == nil ||
		lookup.Binding.ExternalProjectID != binding.ExternalProjectID {
		t.Fatalf("project lookup = %+v", lookup)
	}

	task1 := uuid.New()
	task2 := uuid.New()
	tasks := []annotationapp.EngineTask{
		{
			TaskID: task1, SourceItemRef: "row:1", SourceSHA256: strings.Repeat("b", 64),
			TaskText: "first evidence row", TaskTextSHA256: strings.Repeat("c", 64),
			CorrelationKey: "task-" + task1.String(),
		},
		{
			TaskID: task2, SourceItemRef: "row:2", SourceSHA256: strings.Repeat("d", 64),
			TaskText: "second evidence row", TaskTextSHA256: strings.Repeat("e", 64),
			CorrelationKey: "task-" + task2.String(),
		},
	}
	submitRequest := annotationapp.EngineSubmitRequest{
		WorkspaceID:        workspaceID,
		CampaignID:         campaignID,
		Binding:            binding,
		RequestID:          "submit-" + uuid.NewString(),
		RequestFingerprint: strings.Repeat("f", 64),
		Tasks:              tasks,
	}
	submission, err := client.SubmitTasks(context.Background(), submitRequest)
	if err != nil {
		t.Fatalf("submit tasks: %v", err)
	}
	if submission.State != annotationapp.EngineLookupMatched || len(submission.ExternalTaskIDs) != 2 {
		t.Fatalf("submission = %+v", submission)
	}

	reconciled, err := client.LookupSubmission(context.Background(), annotationapp.EngineLookupRequest{
		WorkspaceID:        submitRequest.WorkspaceID,
		CampaignID:         submitRequest.CampaignID,
		Binding:            submitRequest.Binding,
		RequestID:          submitRequest.RequestID,
		RequestFingerprint: submitRequest.RequestFingerprint,
		Tasks:              submitRequest.Tasks,
	})
	if err != nil {
		t.Fatalf("lookup tasks: %v", err)
	}
	if reconciled.State != annotationapp.EngineLookupMatched ||
		reconciled.ExternalTaskIDs[task1] != submission.ExternalTaskIDs[task1] ||
		reconciled.ExternalTaskIDs[task2] != submission.ExternalTaskIDs[task2] {
		t.Fatalf("task lookup = %+v", reconciled)
	}

	createAnnotation(
		t,
		httpClient,
		baseURL,
		accessToken,
		submission.ExternalTaskIDs[task1],
		"EVIDENCE_SUFFICIENT",
	)

	page, err := client.FetchResults(context.Background(), annotationapp.EngineLookupRequest{
		WorkspaceID:        submitRequest.WorkspaceID,
		CampaignID:         submitRequest.CampaignID,
		Binding:            submitRequest.Binding,
		RequestID:          submitRequest.RequestID,
		RequestFingerprint: submitRequest.RequestFingerprint,
		Tasks:              submitRequest.Tasks,
	}, annotationapp.EngineResultCursor{})
	if err != nil {
		t.Fatalf("fetch results: %v", err)
	}
	var found bool
	for _, result := range page.Results {
		if result.TaskID != task1 {
			continue
		}
		found = true
		if got := string(result.CanonicalPayload); got != `{"label":"EVIDENCE_SUFFICIENT"}` {
			t.Fatalf("canonical payload = %s", got)
		}
		if result.ExternalTaskID != submission.ExternalTaskIDs[task1] ||
			result.ExternalAnnotationID == "" ||
			result.ExternalAuthorRef == "" ||
			result.CanonicalPayloadSHA256 != sha256HexString(`{"label":"EVIDENCE_SUFFICIENT"}`) {
			t.Fatalf("normalized result = %+v", result)
		}
	}
	if !found {
		t.Fatalf("live Label Studio result for Core task %s was not observed: %+v", task1, page.Results)
	}
}

func refreshPersonalAccessToken(
	t *testing.T,
	client *http.Client,
	baseURL, refreshToken string,
) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"refresh": refreshToken})
	if err != nil {
		t.Fatalf("marshal PAT refresh request: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/api/token/refresh",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create PAT refresh request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("refresh Label Studio PAT: %v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("refresh Label Studio PAT status=%d body=%s", response.StatusCode, string(raw))
	}
	var payload struct {
		Access string `json:"access"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || strings.TrimSpace(payload.Access) == "" {
		t.Fatalf("decode Label Studio access token: %v body=%s", err, string(raw))
	}
	return strings.TrimSpace(payload.Access)
}

func createAnnotation(
	t *testing.T,
	client *http.Client,
	baseURL, token, taskID, label string,
) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"result": []any{
			map[string]any{
				"from_name": "label",
				"to_name":   "text",
				"type":      "choices",
				"value": map[string]any{
					"choices": []string{label},
				},
			},
		},
		"was_cancelled": false,
	})
	if err != nil {
		t.Fatalf("marshal live annotation: %v", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/api/tasks/"+taskID+"/annotations/",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create annotation request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("create live annotation: %v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("create live annotation status=%d body=%s", response.StatusCode, string(raw))
	}
}

func deleteProject(t *testing.T, client *http.Client, baseURL, token, projectID string) {
	t.Helper()
	request, err := http.NewRequest(
		http.MethodDelete,
		strings.TrimRight(baseURL, "/")+"/api/projects/"+projectID,
		nil,
	)
	if err != nil {
		t.Logf("create project cleanup request: %v", err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Logf("delete live Label Studio project: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		t.Logf("delete live Label Studio project status=%d body=%s", response.StatusCode, string(raw))
	}
}

func sha256HexString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

var _ = fmt.Sprintf
