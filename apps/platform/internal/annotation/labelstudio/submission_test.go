package labelstudio

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestPythonSnapshotGoldenVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/python-snapshot-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []struct{ Raw, Canonical, SHA256 string }
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, v := range fixture.Vectors {
		canonical, err := PythonSnapshotJSON([]byte(v.Raw))
		if err != nil || string(canonical) != v.Canonical || d.SourceDigest(canonical) != v.SHA256 {
			t.Fatalf("CPython vector %s: canonical=%s hash=%s err=%v", v.Raw, canonical, d.SourceDigest(canonical), err)
		}
	}
	for _, raw := range []string{`{"a":1,"a":2}`, `"\ud800"`, `"\udc00"`, `1e1000`, `{} {}`} {
		if _, err := PythonSnapshotJSON([]byte(raw)); err == nil {
			t.Fatalf("invalid snapshot accepted %s", raw)
		}
	}
}
func controlledAdapterFixture(t *testing.T) (app.EngineLookupRequest, immutableSubmission) {
	t.Helper()
	text := "中文 <&>"
	task := app.EngineTask{TaskID: uuid.New(), SourceItemRef: "row:1", SourceSHA256: strings.Repeat("1", 64), TaskText: text, TaskTextSHA256: d.SourceDigest([]byte(text)), CorrelationKey: "correlation"}
	config, hash, err := labelConfigFromSchema(`{"kind":"single-label-v1","labels":["A","B"]}`)
	if err != nil {
		t.Fatal(err)
	}
	contract := d.SourceContract{AdmissionProtocol: d.ControlledSubmissionProtocol, ConnectionID: uuid.New(), ProviderIncarnation: "isolated-adapter-test", SourceCommit: strings.Repeat("1", 40), EngineVersion: "synthetic-294", ImageDigest: "sha256:" + strings.Repeat("2", 64), NormalizerVersion: "label-studio-single-label-v1"}
	req := app.EngineLookupRequest{WorkspaceID: uuid.New(), CampaignID: uuid.New(), RequestID: "request", RequestFingerprint: strings.Repeat("3", 64), Tasks: []app.EngineTask{task}, Binding: app.EngineCampaignBinding{SourceContract: contract, Provider: Provider, ProviderInstance: "test", ExternalProjectID: "41", ConfigSHA256: hash}}
	snapshot := map[string]any{"annotation": map[string]any{"id": 201, "was_cancelled": false, "result": []any{map[string]any{"from_name": "label", "to_name": "text", "type": "choices", "value": map[string]any{"choices": []string{"A"}}}}},
		"task":    map[string]any{"id": 101, "data": map[string]any{"text": text}, "meta": map[string]any{"core_task_id": task.TaskID.String(), "core_request_id": req.RequestID, "core_request_fingerprint": req.RequestFingerprint, "core_source_item_ref": task.SourceItemRef, "core_source_sha256": task.SourceSHA256, "core_task_text_sha256": task.TaskTextSHA256, "core_correlation_key": task.CorrelationKey}},
		"project": map[string]any{"id": 41, "label_config": config}}
	raw, _ := json.Marshal(snapshot)
	canonical, err := PythonSnapshotJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	source := immutableSubmission{ID: "501", Assignment: "301", Annotation: "201", Revision: 1, Snapshot: canonical, ResultHash: d.SourceDigest(canonical), Status: "PENDING", Review: json.RawMessage("null")}
	source.SubmittedBy.ID = "7"
	return req, source
}

func TestControlledSubmissionOrdinaryListDetailAndFailures(t *testing.T) {
	for _, scenario := range []string{"valid", "fork-review", "author-drift", "body-drift", "bad-hash", "unauthorized", "missing-api", "exact-config", "no-observer", "downgrade", "pagination"} {
		t.Run(scenario, func(t *testing.T) {
			req, source := controlledAdapterFixture(t)
			var requests, starts, finishes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" {
					t.Errorf("non-read request %s", r.Method)
				}
				if strings.Contains(r.URL.Path, "review") || strings.Contains(r.URL.Path, "release") {
					t.Errorf("fork business approval endpoint used")
				}
				if r.Header.Get("Authorization") != "Token test-token" {
					t.Errorf("missing provider auth")
				}
				write := func(v any) {
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(v); err != nil {
						t.Error(err)
					}
				}
				switch {
				case r.URL.Path == "/api/projects/41":
					var body struct {
						Project struct {
							LabelConfig string `json:"label_config"`
						} `json:"project"`
					}
					_ = json.Unmarshal(source.Snapshot, &body)
					config := body.Project.LabelConfig
					if scenario == "exact-config" {
						config = " " + config
					}
					write(map[string]any{"id": 41, "label_config": config})
				case r.URL.Path == "/api/submissions/":
					if scenario == "unauthorized" {
						w.WriteHeader(403)
						return
					}
					if scenario == "missing-api" {
						w.WriteHeader(404)
						return
					}
					listed := source
					if scenario == "fork-review" {
						listed.Status = "APPROVED"
					}
					if scenario == "bad-hash" {
						listed.ResultHash = strings.Repeat("f", 64)
					}
					count := 1
					results := []immutableSubmission{listed}
					var next any
					if scenario == "pagination" {
						count = 101
						page, _ := strconv.Atoi(r.URL.Query().Get("page"))
						results = nil
						if page == 1 {
							for i := 0; i < 100; i++ {
								s := listed
								s.ID = json.Number(strconv.Itoa(501 + i))
								results = append(results, s)
							}
							next = "untrusted-url"
						} else {
							s := listed
							s.ID = "601"
							results = append(results, s)
						}
					}
					write(map[string]any{"count": count, "next": next, "results": results})
				case strings.HasPrefix(r.URL.Path, "/api/submissions/"):
					exact := source
					id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/submissions/"), "/")
					exact.ID = json.Number(id)
					if scenario == "fork-review" {
						exact.Status = "APPROVED"
					}
					if scenario == "author-drift" {
						exact.SubmittedBy.ID = "8"
					}
					if scenario == "body-drift" {
						var m map[string]any
						_ = json.Unmarshal(exact.Snapshot, &m)
						m["extra"] = "different raw same label"
						raw, _ := json.Marshal(m)
						exact.Snapshot, _ = PythonSnapshotJSON(raw)
						exact.ResultHash = d.SourceDigest(exact.Snapshot)
					}
					if scenario == "bad-hash" {
						exact.ResultHash = strings.Repeat("f", 64)
					}
					write(exact)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client, err := NewControlledClient(server.URL, "test-token", "test", req.Binding.SourceContract, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			observer := &app.EngineInvocationObserver{Start: func(context.Context) (d.EngineAttempt, error) { starts++; return d.EngineAttempt{ID: uuid.New()}, nil }, Finish: func(context.Context, d.EngineAttempt, error) error { finishes++; return nil }}
			ctx := app.WithEngineInvocationObserver(t.Context(), observer)
			if scenario == "no-observer" {
				ctx = t.Context()
			}
			if scenario == "downgrade" {
				req.Binding.SourceContract = d.SourceContract{}
			}
			page, err := client.FetchResults(ctx, req, app.EngineResultCursor{})
			valid := scenario == "valid" || scenario == "fork-review" || scenario == "pagination"
			if valid && err != nil {
				t.Fatal(err)
			}
			if !valid && err == nil {
				t.Fatalf("%s silently accepted", scenario)
			}
			if scenario == "valid" || scenario == "fork-review" {
				if len(page.Results) != 1 || page.Results[0].Source == nil || page.Results[0].ExternalAuthorRef != "7" || page.Results[0].Source.PhysicalAttemptID == uuid.Nil {
					t.Fatalf("observation %+v", page)
				}
				if page.Results[0].Quarantined != (scenario == "fork-review") {
					t.Fatal("review contamination not quarantined")
				}
			}
			if scenario == "pagination" {
				if len(page.Results) != 100 || page.NextCursor == nil {
					t.Fatal("first page incomplete")
				}
				second, err := client.FetchResults(ctx, req, *page.NextCursor)
				if err != nil || len(second.Results) != 1 || second.NextCursor != nil {
					t.Fatalf("last page %+v %v", second, err)
				}
			}
			if starts != requests || finishes != starts {
				t.Fatalf("physical attempts starts/http/outcomes=%d/%d/%d", starts, requests, finishes)
			}
		})
	}
}
