package labelstudio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func NewControlledClient(baseURL, token, instanceRef string, contract annotationdomain.SourceContract, httpClient *http.Client) (*Client, error) {
	if contract.Protocol() != annotationdomain.ControlledSubmissionProtocol || contract.NormalizerVersion != "label-studio-single-label-v1" {
		return nil, annotationdomain.ErrSourceIntegrity
	}
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	c, err := NewClient(baseURL, token, instanceRef, httpClient)
	if err != nil {
		return nil, err
	}
	configuredClient := *c.httpClient
	configuredClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.httpClient = &configuredClient
	c.sourceContract = contract
	return c, nil
}

type immutableSubmission struct {
	ID          json.Number     `json:"id"`
	Assignment  json.Number     `json:"assignment"`
	Annotation  json.Number     `json:"annotation"`
	Revision    int64           `json:"revision"`
	Snapshot    json.RawMessage `json:"result_snapshot"`
	ResultHash  string          `json:"result_hash"`
	SubmittedBy struct {
		ID json.Number `json:"id"`
	} `json:"submitted_by"`
	Status string          `json:"status"`
	Review json.RawMessage `json:"review"`
}

func validProviderID(n json.Number) bool {
	s := n.String()
	if s == "" || s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func (c *Client) fetchSubmissions(ctx context.Context, req annotationapp.EngineLookupRequest, cursor annotationapp.EngineResultCursor) (annotationapp.EngineResultPage, error) {
	fail := func() (annotationapp.EngineResultPage, error) {
		return annotationapp.EngineResultPage{}, annotationapp.ErrAnnotationEngineInvalidResponse
	}
	expected := req.Binding.SourceContract
	actual := c.sourceContract
	expected.AdmissionProtocol = expected.Protocol()
	actual.AdmissionProtocol = actual.Protocol()
	if expected != actual || expected.Protocol() != annotationdomain.ControlledSubmissionProtocol || cursor.Offset < 0 || cursor.Offset%100 != 0 {
		return fail()
	}
	if err := validateBinding(c.instanceRef, req.Binding); err != nil {
		return annotationapp.EngineResultPage{}, err
	}
	if err := c.verifyProjectConfig(ctx, req.Binding); err != nil {
		return annotationapp.EngineResultPage{}, err
	}
	query := url.Values{"project": {req.Binding.ExternalProjectID}, "page": {strconv.Itoa(cursor.Offset/100 + 1)}, "page_size": {"100"}}
	var page struct {
		Count   *int                  `json:"count"`
		Next    json.RawMessage       `json:"next"`
		Results []immutableSubmission `json:"results"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, "/api/submissions/", query, nil, &page); err != nil {
		return annotationapp.EngineResultPage{}, err
	}
	if page.Count == nil || *page.Count < 0 || len(page.Results) > 100 || cursor.Offset+len(page.Results) > *page.Count {
		return fail()
	}
	result := annotationapp.EngineResultPage{}
	seen := map[string]bool{}
	for _, listed := range page.Results {
		if !validProviderID(listed.ID) || seen[listed.ID.String()] {
			return fail()
		}
		seen[listed.ID.String()] = true
		var exact immutableSubmission
		if err := c.requestJSON(ctx, http.MethodGet, "/api/submissions/"+url.PathEscape(listed.ID.String())+"/", nil, nil, &exact); err != nil {
			return annotationapp.EngineResultPage{}, err
		}
		observation, err := c.normalizeSubmission(req, exact)
		if err != nil {
			return annotationapp.EngineResultPage{}, err
		}
		listedObservation, err := c.normalizeSubmission(req, listed)
		if err != nil {
			return annotationapp.EngineResultPage{}, err
		}
		if exact.ID != listed.ID || exact.Assignment != listed.Assignment || exact.Revision != listed.Revision ||
			exact.Annotation != listed.Annotation || exact.SubmittedBy.ID != listed.SubmittedBy.ID ||
			!bytes.Equal(observation.Source.Snapshot, listedObservation.Source.Snapshot) ||
			observation.Quarantined != listedObservation.Quarantined {
			return fail()
		}
		if observer := annotationapp.EngineInvocationFromContext(ctx); observer != nil {
			observation.Source.PhysicalAttemptID = observer.LastAttemptID
		}
		// A current write token is observation metadata, not a historical token.
		// Reassignment does not change immutable submitted_by.
		var assignment struct {
			ID      json.Number `json:"id"`
			Task    json.Number `json:"task"`
			Project json.Number `json:"project"`
			Version int64       `json:"version"`
		}
		if err := c.requestJSON(ctx, http.MethodGet, "/api/task-assignments/"+url.PathEscape(exact.Assignment.String())+"/", nil, nil, &assignment); err != nil {
			return annotationapp.EngineResultPage{}, err
		}
		if assignment.ID != exact.Assignment || assignment.Task.String() != observation.ExternalTaskID ||
			assignment.Project.String() != req.Binding.ExternalProjectID || assignment.Version < 1 {
			return fail()
		}
		observation.Source.AssignmentObservation = annotationdomain.AssignmentObservation{Version: assignment.Version, ObservedAt: time.Now().UTC()}
		result.Results = append(result.Results, observation)
	}
	hasNext := len(page.Next) > 0 && !bytes.Equal(bytes.TrimSpace(page.Next), []byte("null")) && string(page.Next) != "\"\""
	if cursor.Offset+len(page.Results) < *page.Count {
		if !hasNext || len(page.Results) != 100 {
			return fail()
		}
		next := annotationapp.EngineResultCursor{Offset: cursor.Offset + 100}
		result.NextCursor = &next
	} else if hasNext {
		return fail()
	}
	return result, nil
}
func (c *Client) normalizeSubmission(req annotationapp.EngineLookupRequest, s immutableSubmission) (annotationapp.EngineResultObservation, error) {
	invalid := func() (annotationapp.EngineResultObservation, error) {
		return annotationapp.EngineResultObservation{}, annotationapp.ErrAnnotationEngineInvalidResponse
	}
	if !validProviderID(s.ID) || !validProviderID(s.Assignment) || !validProviderID(s.Annotation) || !validProviderID(s.SubmittedBy.ID) || s.Revision < 1 {
		return invalid()
	}
	raw, err := PythonSnapshotJSON(s.Snapshot)
	if err != nil || annotationdomain.SourceDigest(raw) != s.ResultHash {
		return invalid()
	}
	var snapshot struct {
		Annotation struct {
			ID        json.Number `json:"id"`
			Result    []any       `json:"result"`
			Cancelled *bool       `json:"was_cancelled"`
		} `json:"annotation"`
		Task struct {
			ID   json.Number    `json:"id"`
			Data map[string]any `json:"data"`
			Meta map[string]any `json:"meta"`
		} `json:"task"`
		Project struct {
			ID          json.Number `json:"id"`
			LabelConfig string      `json:"label_config"`
		} `json:"project"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&snapshot); err != nil {
		return invalid()
	}
	if snapshot.Annotation.ID != s.Annotation || snapshot.Annotation.Cancelled == nil || *snapshot.Annotation.Cancelled || !validProviderID(snapshot.Task.ID) ||
		snapshot.Project.ID.String() != req.Binding.ExternalProjectID ||
		annotationdomain.SourceDigest([]byte(snapshot.Project.LabelConfig)) != req.Binding.ConfigSHA256 {
		return invalid()
	}
	taskID, _ := snapshot.Task.Meta["core_task_id"].(string)
	requestID, _ := snapshot.Task.Meta["core_request_id"].(string)
	requestFingerprint, _ := snapshot.Task.Meta["core_request_fingerprint"].(string)
	if requestID != req.RequestID || requestFingerprint != req.RequestFingerprint {
		return invalid()
	}
	var task *annotationapp.EngineTask
	for i := range req.Tasks {
		if req.Tasks[i].TaskID.String() == taskID {
			task = &req.Tasks[i]
			break
		}
	}
	if task == nil || !remoteTaskMatches(*task, snapshot.Task.Data, snapshot.Task.Meta) {
		return invalid()
	}
	if len(snapshot.Annotation.Result) != 1 {
		return invalid()
	}
	item, ok := snapshot.Annotation.Result[0].(map[string]any)
	if !ok || item["from_name"] != "label" || item["to_name"] != "text" || item["type"] != "choices" {
		return invalid()
	}
	label, ok := singleChoiceLabel(snapshot.Annotation.Result)
	if !ok {
		return invalid()
	}
	payload, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		return invalid()
	}
	quarantine := s.Status == "APPROVED" || s.Status == "REJECTED" ||
		len(bytes.TrimSpace(s.Review)) > 0 && !bytes.Equal(bytes.TrimSpace(s.Review), []byte("null"))
	if s.Status != "PENDING" && s.Status != "SUPERSEDED" && !quarantine {
		return invalid()
	}
	return annotationapp.EngineResultObservation{TaskID: task.TaskID, ExternalTaskID: snapshot.Task.ID.String(),
		ExternalAnnotationID: s.Annotation.String(), ExternalRevision: fmt.Sprintf("submission/%s/assignment/%s/revision/%d", s.ID, s.Assignment, s.Revision),
		ExternalAuthorRef: s.SubmittedBy.ID.String(), CanonicalPayload: payload, CanonicalPayloadSHA256: annotationdomain.SourceDigest(payload),
		NormalizerVersion: "label-studio-single-label-v1", ProviderSubmitted: true, Quarantined: quarantine,
		Source: &annotationapp.EngineImmutableSource{ExternalID: s.ID.String(), AssignmentID: s.Assignment.String(), Revision: s.Revision,
			Snapshot: raw, SnapshotSHA256: s.ResultHash, ExternalProjectID: snapshot.Project.ID.String()}}, nil
}
func (c *Client) validateContract(contract annotationdomain.SourceContract) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	actual := c.sourceContract
	actual.AdmissionProtocol = actual.Protocol()
	contract.AdmissionProtocol = contract.Protocol()
	if actual != contract {
		return annotationapp.ErrAnnotationEngineInvalidRequest
	}
	return nil
}
