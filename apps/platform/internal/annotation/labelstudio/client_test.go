package labelstudio_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/labelstudio"
)

func TestLabelStudioCreateProjectAndSubmitTasks(t *testing.T) {
	var imported []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/projects":
			if r.Method != http.MethodPost {
				t.Fatalf("project method = %s", r.Method)
			}
			writeJSON(t, w, map[string]any{
				"id":           41,
				"label_config": "<View><Choices name=\"label\" toName=\"text\"/></View>",
			})
		case "/api/projects/41/import":
			if r.URL.Query().Get("return_task_ids") != "true" {
				t.Fatalf("return_task_ids = %q", r.URL.Query().Get("return_task_ids"))
			}
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &imported); err != nil {
				t.Fatalf("decode import: %v", err)
			}
			writeJSON(t, w, map[string]any{
				"task_count": 2,
				"task_ids":   []int{101, 102},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := labelstudio.NewClient(server.URL, "secret", "local-ls", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	workspaceID := uuid.New()
	campaignID := uuid.New()
	config := "<View><Choices name=\"label\" toName=\"text\"/></View>"
	binding, err := client.EnsureCampaignBinding(context.Background(), annotationapp.EngineCampaignRequest{
		WorkspaceID:        workspaceID,
		CampaignID:         campaignID,
		RequestID:          "campaign-create-1",
		RequestFingerprint: strings.Repeat("a", 64),
		Title:              "gold-pilot",
		LabelConfig:        config,
		ConfigSHA256:       strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatalf("ensure binding: %v", err)
	}
	if binding.ExternalProjectID != "41" || binding.Provider != labelstudio.Provider {
		t.Fatalf("binding = %+v", binding)
	}

	task1 := uuid.New()
	task2 := uuid.New()
	submitted, err := client.SubmitTasks(context.Background(), annotationapp.EngineSubmitRequest{
		WorkspaceID:        workspaceID,
		CampaignID:         campaignID,
		Binding:            binding,
		RequestID:          "submit-1",
		RequestFingerprint: strings.Repeat("c", 64),
		Tasks: []annotationapp.EngineTask{
			{
				TaskID: task1, SourceItemRef: "row-1", SourceSHA256: strings.Repeat("d", 64),
				TaskText: "first", TaskTextSHA256: strings.Repeat("e", 64), CorrelationKey: "task-1",
			},
			{
				TaskID: task2, SourceItemRef: "row-2", SourceSHA256: strings.Repeat("f", 64),
				TaskText: "second", TaskTextSHA256: strings.Repeat("1", 64), CorrelationKey: "task-2",
			},
		},
	})
	if err != nil {
		t.Fatalf("submit tasks: %v", err)
	}
	if submitted.State != annotationapp.EngineLookupMatched ||
		submitted.ExternalTaskIDs[task1] != "101" ||
		submitted.ExternalTaskIDs[task2] != "102" {
		t.Fatalf("submission = %+v", submitted)
	}
	if len(imported) != 2 {
		t.Fatalf("imported tasks = %d", len(imported))
	}
	meta, ok := imported[0]["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v", imported[0]["meta"])
	}
	if meta["core_task_id"] != task1.String() || meta["core_request_id"] != "submit-1" {
		t.Fatalf("correlation meta = %+v", meta)
	}
}

func TestLabelStudioTransportFailureIsRetryableWithoutLeakingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider secret detail", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := labelstudio.NewClient(server.URL, "secret", "local-ls", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.EnsureCampaignBinding(context.Background(), annotationapp.EngineCampaignRequest{
		WorkspaceID:        uuid.New(),
		CampaignID:         uuid.New(),
		RequestID:          "request",
		RequestFingerprint: strings.Repeat("a", 64),
		Title:              "pilot",
		LabelConfig:        "<View/>",
		ConfigSHA256:       strings.Repeat("b", 64),
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	engineErr, ok := err.(*annotationapp.AnnotationEngineError)
	if !ok || !engineErr.Retryable || engineErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("engine error = %#v", err)
	}
	if strings.Contains(err.Error(), "secret detail") || strings.Contains(err.Error(), "Label Studio") {
		t.Fatalf("provider detail leaked: %q", err.Error())
	}
}

func TestLabelStudioLookupSubmissionReturnsUnknownForPartialMatch(t *testing.T) {
	task1 := uuid.New()
	task2 := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tasks" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{
			"tasks": []any{
				map[string]any{
					"id": 101,
					"meta": map[string]any{
						"core_task_id":             task1.String(),
						"core_request_id":          "submit-1",
						"core_request_fingerprint": "fp-1",
					},
				},
			},
			"next": nil,
		})
	}))
	defer server.Close()

	client, err := labelstudio.NewClient(server.URL, "secret", "local-ls", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	result, err := client.LookupSubmission(context.Background(), annotationapp.EngineLookupRequest{
		WorkspaceID: uuid.New(),
		CampaignID:  uuid.New(),
		Binding: annotationapp.EngineCampaignBinding{
			Provider: ProviderName(),
			ProviderInstance: "local-ls",
			ExternalProjectID: "41",
		},
		RequestID:          "submit-1",
		RequestFingerprint: "fp-1",
		Tasks: []annotationapp.EngineTask{
			{TaskID: task1},
			{TaskID: task2},
		},
	})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if result.State != annotationapp.EngineLookupUnknown || result.ExternalTaskIDs[task1] != "101" {
		t.Fatalf("lookup = %+v", result)
	}
}

func ProviderName() string {
	return labelstudio.Provider
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
