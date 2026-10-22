package sourceadmission

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
)

// HTTPSource has no URL-valued request input, redirect, retry, proxy or fallback.
// The only endpoints are the authenticated #80 Manifest and asset endpoints.
type HTTPSource struct{ Client *http.Client }

func NewHTTPSource() *HTTPSource {
	// One client.Do must be one durable invocation. HTTP/1 on a fresh connection
	// avoids the transport's implicit GET retry after a reused connection fails.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	return &HTTPSource{Client: &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil, Protocols: protocols, DisableKeepAlives: true, ResponseHeaderTimeout: 15 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func sourceBase(c Connection) (*url.URL, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") {
		return nil, ErrDenied
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || (u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return nil, ErrDenied
		}
	}
	return u, nil
}
func (s *HTTPSource) Fetch(ctx context.Context, service *Service, operation uuid.UUID, r Request, c Connection, name, asset string) ([]byte, error) {
	u, e := sourceBase(c)
	if e != nil || s == nil || s.Client == nil {
		return nil, ErrDenied
	}
	token, e := os.ReadFile(c.TokenFile)
	if e != nil || len(token) > 16384 || strings.TrimSpace(string(token)) == "" || strings.ContainsAny(strings.TrimSpace(string(token)), "\r\n") {
		return nil, ErrDenied
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/projects/" + c.Project + "/approved-dataset-releases/" + r.ReleaseID.String() + "/snapshots/" + r.SnapshotID.String() + "/"
	if name == "manifest.json" {
		u.Path += "manifest/"
	} else {
		if _, e = uuid.Parse(asset); e != nil {
			return nil, ErrIntegrity
		}
		u.Path += "assets/" + asset + "/download/"
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return nil, ErrDenied
	}
	req.Header.Set("Authorization", "Token "+strings.TrimSpace(string(token)))
	req.Header.Set("Accept", "application/json")
	call := uuid.New()
	at := time.Now().UTC()
	// This records an actual transport attempt, not a known charge or success.
	e = service.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO source_admission_call(id,operation_id,workspace_id,asset_name,started_at) VALUES($1,$2,$3,$4,$5)`, call, operation, r.WorkspaceID, name, at); e != nil {
			return e
		}
		ce := uuid.New()
		if e := cost.Append(ctx, tx, cost.Event{ID: ce, WorkspaceID: r.WorkspaceID, ActivityID: call, CostType: "SOURCE_ADMISSION_FETCH", Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL", OccurredAt: at}); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,source_admission_call_id) VALUES($1,$2,$3)`, uuid.New(), ce, call)
		return e
	})
	if e != nil {
		return nil, e
	}
	client := *s.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, callErr := client.Do(req)
	status := 0
	outcome := "UNKNOWN"
	var data []byte
	resultErr := ErrUnknown
	if response != nil {
		status = response.StatusCode
		if status == http.StatusOK {
			data, e = io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
			if len(data) > MaxBytes {
				outcome = "INVALID"
				resultErr = ErrIntegrity
			} else if e != nil {
				// A short/failed transport read is not a verified byte mismatch.
				outcome = "UNKNOWN"
				resultErr = ErrUnknown
			} else {
				outcome = "RECEIVED"
				resultErr = nil
			}
		} else if status == 401 || status == 403 {
			outcome = "DENIED"
			resultErr = ErrDenied
		} else if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests {
			outcome = "UNKNOWN"
			resultErr = ErrUnknown
		} else if status < 500 {
			outcome = "INVALID"
			resultErr = ErrIntegrity
		}
		if e = response.Body.Close(); e != nil && resultErr == nil {
			resultErr = ErrUnknown
			outcome = "UNKNOWN"
		}
	}
	if callErr != nil {
		resultErr = ErrUnknown
		outcome = "UNKNOWN"
	}
	// A failed observation append cannot turn a possibly sent call into ABSENT.
	_, e = service.Pool.Exec(context.WithoutCancel(ctx), `INSERT INTO source_admission_observation(call_id,outcome,status_code) VALUES($1,$2,$3)`, call, outcome, status)
	if e != nil {
		return nil, errors.Join(ErrUnknown, e)
	}
	return data, resultErr
}
