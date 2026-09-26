package hop_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
)

func TestManagedHopLifecycle(t *testing.T) {
	const (
		pipelineName = "enterprise-activity-hop"
		runID        = "hop-run-001"
	)
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "cluster" || password != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/hop/registerPipeline":
			if r.Method != http.MethodPost || r.URL.Query().Get("xml") != "Y" {
				t.Fatalf("register request = %s %s", r.Method, r.URL.String())
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "<pipeline_configuration>") {
				t.Fatalf("register body = %s", string(body))
			}
			writeXML(w, fmt.Sprintf(`<webresult><result>OK</result><message>registered</message><id>%s</id></webresult>`, runID))
		case "/hop/startPipeline":
			assertRunQuery(t, r.URL.Query(), pipelineName, runID)
			if r.URL.Query().Get("TARGET_PERIOD") != "2025-03" || r.URL.Query().Get("xml") != "Y" {
				t.Fatalf("start query = %v", r.URL.Query())
			}
			writeXML(w, `<webresult><result>OK</result><message>started</message><id></id></webresult>`)
		case "/hop/pipelineStatus":
			assertRunQuery(t, r.URL.Query(), pipelineName, runID)
			if r.URL.Query().Get("json") != "Y" {
				t.Fatalf("status query missing json=Y: %v", r.URL.Query())
			}
			statusCalls++
			from := r.URL.Query().Get("from")
			logging := ""
			firstLine := 0
			lastLine := 0
			if from == "5" {
				logging = encodeHopLog(t, "line five\nline six\n")
				firstLine = 5
				lastLine = 6
			}
			writeJSON(t, w, map[string]any{
				"id":                  runID,
				"pipelineName":        pipelineName,
				"statusDescription":   "Running",
				"errorDescription":    "",
				"loggingString":       logging,
				"firstLoggingLineNr":  firstLine,
				"lastLoggingLineNr":   lastLine,
				"executionStartDate":  "2026-09-17T01:00:00.000+0000",
				"executionEndDate":    nil,
				"paused":              false,
				"transformStatusList": []any{map[string]any{"transformName": "aggregate", "errors": 0}},
				"result":              map[string]any{"nrErrors": 0, "result": true},
			})
		case "/hop/stopPipeline":
			assertRunQuery(t, r.URL.Query(), pipelineName, runID)
			if r.URL.Query().Get("xml") != "Y" {
				t.Fatalf("stop query missing xml=Y: %v", r.URL.Query())
			}
			writeXML(w, `<webresult><result>OK</result><message>stopped</message><id></id></webresult>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := hop.NewClient(server.URL+"/hop", "cluster", "secret", server.Client())
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}
	run, err := client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:          pipelineName,
		DefinitionRef: "examples/enterprise-activity/hop/aggregate.hpl",
		Definition:    []byte(`<pipeline_configuration><pipeline><name>enterprise-activity-hop</name></pipeline></pipeline_configuration>`),
		ContentType:   "application/xml",
		Parameters:    map[string]string{"TARGET_PERIOD": "2025-03"},
	})
	if err != nil {
		t.Fatalf("submit Hop run: %v", err)
	}
	if run.ID != runID || run.State != workflowapp.EngineRunRunning || run.StartedAt == nil {
		t.Fatalf("submitted run = %+v", run)
	}
	if run.Metrics["definitionRef"] != "examples/enterprise-activity/hop/aggregate.hpl" {
		t.Fatalf("definition ref missing from run metrics: %+v", run.Metrics)
	}

	logs, err := client.Logs(context.Background(), pipelineName, runID, 5)
	if err != nil {
		t.Fatalf("read Hop logs: %v", err)
	}
	if logs.Text != "line five\nline six\n" || logs.From != 5 || logs.NextOffset != 7 {
		t.Fatalf("logs = %+v", logs)
	}

	metrics, err := client.Metrics(context.Background(), pipelineName, runID)
	if err != nil {
		t.Fatalf("read Hop metrics: %v", err)
	}
	if metrics["nrErrors"] != int64(0) || metrics["transformCount"] != 1 {
		t.Fatalf("metrics = %+v", metrics)
	}

	if err := client.Cancel(context.Background(), pipelineName, runID); err != nil {
		t.Fatalf("cancel Hop run: %v", err)
	}
	if statusCalls < 3 {
		t.Fatalf("status calls = %d, want submit probe + logs + metrics", statusCalls)
	}
}

func TestHopStatusMapsTerminalStates(t *testing.T) {
	tests := []struct {
		name        string
		description string
		nrErrors    int
		want        workflowapp.EngineRunState
	}{
		{name: "success", description: "Finished", nrErrors: 0, want: workflowapp.EngineRunSucceeded},
		{name: "finished errors", description: "Finished with errors", nrErrors: 2, want: workflowapp.EngineRunFailed},
		{name: "cancelled", description: "Stopped", nrErrors: 0, want: workflowapp.EngineRunCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(t, w, map[string]any{
					"id":                 "run-1",
					"pipelineName":       "pipeline-1",
					"statusDescription":  test.description,
					"executionStartDate": "2026-09-17T01:00:00.000+0000",
					"executionEndDate":   "2026-09-17T01:01:00.000+0000",
					"result":             map[string]any{"nrErrors": test.nrErrors},
				})
			}))
			defer server.Close()
			client, err := hop.NewClient(server.URL, "cluster", "secret", server.Client())
			if err != nil {
				t.Fatalf("create Hop client: %v", err)
			}
			run, err := client.Status(context.Background(), "pipeline-1", "run-1")
			if err != nil {
				t.Fatalf("get status: %v", err)
			}
			if run.State != test.want || run.FinishedAt == nil {
				t.Fatalf("run = %+v, want state %s with finish time", run, test.want)
			}
		})
	}
}

func TestHopAdapterMapsProviderFailuresToPlatformCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hop/registerPipeline":
			writeXML(w, `<webresult><result>ERROR</result><message>invalid pipeline secret detail</message><id></id></webresult>`)
		default:
			http.Error(w, "server unavailable secret detail", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	client, err := hop.NewClient(server.URL, "cluster", "secret", server.Client())
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}
	_, err = client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:       "broken",
		Definition: []byte(`<pipeline_configuration/>`),
	})
	assertManagedEngineError(t, err, workflowapp.ManagedEngineRejected, false, 0)
	if strings.Contains(err.Error(), "invalid pipeline secret detail") || strings.Contains(err.Error(), "Hop") {
		t.Fatalf("platform error leaked provider detail: %q", err.Error())
	}

	_, err = client.Status(context.Background(), "broken", "run-x")
	assertManagedEngineError(t, err, workflowapp.ManagedEngineUnavailable, true, http.StatusServiceUnavailable)
	if strings.Contains(err.Error(), "server unavailable secret detail") || strings.Contains(err.Error(), "Hop") {
		t.Fatalf("platform error leaked provider response: %q", err.Error())
	}
}

func TestHopSubmitRejectsReservedRunIdentityParametersBeforeRemoteCall(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := hop.NewClient(server.URL, "cluster", "secret", server.Client())
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}
	for _, reserved := range []string{"name", "ID", " xml "} {
		_, err := client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
			Name:       "pipeline",
			Definition: []byte(`<pipeline_configuration/>`),
			Parameters: map[string]string{reserved: "attacker-controlled"},
		})
		assertManagedEngineError(t, err, workflowapp.ManagedEngineInvalidRequest, false, 0)
	}
	if requests != 0 {
		t.Fatalf("reserved parameter validation made %d remote requests, want 0", requests)
	}
}

func TestHopBaseURLPreservesHostnameNamedHop(t *testing.T) {
	var gotHost, gotPath string
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotHost = req.URL.Host
		gotPath = req.URL.Path
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Request:    req,
		}, nil
	})}
	client, err := hop.NewClient("http://hop", "cluster", "secret", httpClient)
	if err != nil {
		t.Fatalf("create Hop client with hostname hop: %v", err)
	}
	_, _ = client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:       "pipeline",
		Definition: []byte(`<pipeline_configuration/>`),
	})
	if gotHost != "hop" || gotPath != "/hop/registerPipeline" {
		t.Fatalf("request target = host %q path %q, want hop /hop/registerPipeline", gotHost, gotPath)
	}
}

func assertManagedEngineError(t *testing.T, err error, kind workflowapp.ManagedEngineErrorKind, retryable bool, status int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected managed engine error")
	}
	var managed *workflowapp.ManagedEngineError
	if !errors.As(err, &managed) {
		t.Fatalf("error = %T %v, want ManagedEngineError", err, err)
	}
	if managed.Kind != kind || managed.Retryable != retryable || managed.StatusCode != status {
		t.Fatalf("managed error = %+v, want kind=%s retryable=%t status=%d", managed, kind, retryable, status)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func assertRunQuery(t *testing.T, query url.Values, name, runID string) {
	t.Helper()
	if query.Get("name") != name || query.Get("id") != runID {
		t.Fatalf("run query = %v, want name=%s id=%s", query, name, runID)
	}
}

func encodeHopLog(t *testing.T, text string) string {
	t.Helper()
	var compressed bytes.Buffer
	base64Writer := base64.NewEncoder(base64.StdEncoding, &compressed)
	gzipWriter := gzip.NewWriter(base64Writer)
	if _, err := gzipWriter.Write([]byte(text)); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := base64Writer.Close(); err != nil {
		t.Fatalf("close base64: %v", err)
	}
	return compressed.String()
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/xml")
	_, _ = io.WriteString(w, body)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}


func TestHopSubmitClassifiesTransportFailureAsOutcomeUnknown(t *testing.T) {
	client, err := hop.NewClient("http://hop", "cluster", "secret", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("response lost after request write")
		}),
	})
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}

	_, err = client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:       "pipeline",
		Definition: []byte(`<pipeline_configuration/>`),
	})
	assertManagedEngineError(t, err, workflowapp.ManagedEngineOutcomeUnknown, true, 0)
}

func TestHopSubmitClassifiesStartServiceFailureAsOutcomeUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hop/registerPipeline":
			writeXML(w, `<webresult><result>OK</result><message>registered</message><id>run-unknown</id></webresult>`)
		case "/hop/startPipeline":
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := hop.NewClient(server.URL, "cluster", "secret", server.Client())
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}
	_, err = client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:       "pipeline",
		Definition: []byte(`<pipeline_configuration/>`),
	})
	assertManagedEngineError(t, err, workflowapp.ManagedEngineOutcomeUnknown, true, http.StatusServiceUnavailable)
}

func TestHopSubmitTreatsSuccessfulRegistrationWithoutDurableIDAsOutcomeUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hop/registerPipeline" {
			http.NotFound(w, r)
			return
		}
		writeXML(w, `<webresult><result>OK</result><message>registered</message><id></id></webresult>`)
	}))
	defer server.Close()

	client, err := hop.NewClient(server.URL, "cluster", "secret", server.Client())
	if err != nil {
		t.Fatalf("create Hop client: %v", err)
	}
	_, err = client.Submit(context.Background(), workflowapp.ManagedSubmitRequest{
		Name:       "pipeline",
		Definition: []byte(`<pipeline_configuration/>`),
	})
	assertManagedEngineError(t, err, workflowapp.ManagedEngineOutcomeUnknown, true, 0)
}
