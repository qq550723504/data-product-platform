package main

// Synthetic source adapter is confined to this acceptance executable. The
// application has no real source adapter, log collection, or source enable flag.
import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func fixtureCapabilities(c config, name, cap string) []string {
	if c.RuntimeLogs && name != "ordinary" && name != "admin-b" {
		return []string{cap, principal.CapabilityRuntimeLogRead}
	}
	return []string{cap}
}

type syntheticLogSource struct {
	client          *http.Client
	url, credential string
}

func runtimeSources(c config, client *http.Client) management.RuntimeSources {
	if !c.RuntimeLogs {
		return nil
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &syntheticLogSource{&copy, c.Issuer + "/_runtime-logs", c.ReadCredential}
}
func (s *syntheticLogSource) Lookup(_ context.Context, ws uuid.UUID) (management.RuntimeBinding, bool) {
	if ws != wsA && ws != wsB {
		return management.RuntimeBinding{}, false
	}
	return management.RuntimeBinding{ID: "synthetic-" + ws.String(), Version: "v1", RuleVersion: "public-v1", Source: "SYNTHETIC_FIXTURE", Services: []string{"APP_SERVICE", "BACKGROUND_SERVICE"}, Reader: s}, true
}

type nativeLog struct {
	Workspace uuid.UUID  `json:"workspaceId"`
	ID        string     `json:"id"`
	At        time.Time  `json:"recordedAt"`
	Occurred  *time.Time `json:"occurredAt"`
	Service   string     `json:"service"`
	Level     string     `json:"level"`
	Event     string     `json:"eventCode"`
}

func nativeRecord(n nativeLog, ws uuid.UUID) (management.RuntimeRecord, error) {
	if n.Workspace != ws {
		return management.RuntimeRecord{}, management.LogUnavailable
	}
	return management.RuntimeRecord{ID: n.ID, RecordedAt: n.At.UTC(), OccurredAt: n.Occurred, Service: n.Service, Level: n.Level, EventCode: n.Event}, nil
}
func (s *syntheticLogSource) get(ctx context.Context, q url.Values, out any) error {
	r, e := http.NewRequestWithContext(ctx, "GET", s.url+"?"+q.Encode(), nil)
	if e != nil {
		return management.LogUnavailable
	}
	r.Header.Set("Authorization", "Bearer "+s.credential)
	response, e := s.client.Do(r)
	if e != nil {
		return management.LogUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return management.LogMissing
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != 200 || media != "application/json" {
		return management.LogUnavailable
	}
	payload, e := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if e != nil || len(payload) > 256<<10 {
		return management.LogUnavailable
	}
	var raw json.RawMessage
	if json.Unmarshal(payload, &raw) != nil {
		return management.LogUnavailable
	}
	// Strict decoder rejects free provider fields instead of forwarding them.
	d := json.NewDecoder(bytes.NewReader(payload))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return management.LogUnavailable
	}
	return nil
}
func (s *syntheticLogSource) Query(ctx context.Context, ws uuid.UUID, q management.RuntimeQuery) (management.RuntimePage, error) {
	values := url.Values{"workspaceId": {ws.String()}, "start": {q.Start.Format(management.LogTimeLayout)}, "end": {q.End.Format(management.LogTimeLayout)}, "asOf": {q.AsOf.Format(management.LogTimeLayout)}}
	if q.Service != "" {
		values.Set("service", q.Service)
	}
	if q.Level != "" {
		values.Set("level", q.Level)
	}
	if q.After != nil {
		values.Set("afterTime", q.After.At.Format(management.LogTimeLayout))
		values.Set("afterId", q.After.ID)
	}
	var data struct {
		Items    []nativeLog `json:"items"`
		Complete bool        `json:"complete"`
	}
	if e := s.get(ctx, values, &data); e != nil {
		return management.RuntimePage{}, e
	}
	if !data.Complete || data.Items == nil || len(data.Items) > 26 {
		return management.RuntimePage{}, management.LogUnavailable
	}
	page := management.RuntimePage{Items: []management.RuntimeRecord{}, Complete: true}
	for _, n := range data.Items {
		r, e := nativeRecord(n, ws)
		if e != nil {
			return management.RuntimePage{}, e
		}
		page.Items = append(page.Items, r)
	}
	if len(page.Items) == 26 {
		page.Items = page.Items[:25]
		last := page.Items[24]
		page.Next = &management.RuntimePosition{At: last.RecordedAt, ID: last.ID}
	}
	return page, nil
}
func (s *syntheticLogSource) ReadExact(ctx context.Context, ws uuid.UUID, id string) (management.RuntimeRecord, error) {
	var n nativeLog
	if e := s.get(ctx, url.Values{"workspaceId": {ws.String()}, "id": {id}}, &n); e != nil {
		return management.RuntimeRecord{}, e
	}
	return nativeRecord(n, ws)
}
