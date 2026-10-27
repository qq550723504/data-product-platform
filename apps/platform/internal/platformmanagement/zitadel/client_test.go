// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package zitadel

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"strings"
	"testing"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

const validResponse = `{"pagination":{"totalResult":"1","appliedLimit":"20"},"authorizations":[{"id":"grant-b","creationDate":"2026-09-12T00:00:00Z","changeDate":"2026-09-12T00:00:00Z","project":{"id":"project"},"organization":{"id":"effective-b"},"user":{"id":"user-a","organizationId":"home-a","displayName":"Member A","preferredLoginName":"a@example.invalid"},"state":"STATE_ACTIVE","roles":[{"key":"listingkit_viewer"}]}]}`

func TestDirectoryTransportRequiresVerifiedIsolatedTLS(t *testing.T) {
	for _, transport := range []http.RoundTripper{
		&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		&http.Transport{TLSClientConfig: &tls.Config{ServerName: "foreign.example.invalid"}},
		&http.Transport{DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("custom TLS dial reached")
			return nil, nil
		}},
		&http.Transport{DialTLS: func(string, string) (net.Conn, error) { t.Fatal("custom TLS dial reached"); return nil, nil }},
		transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unsafe transport reached"); return nil, nil }),
	} {
		if _, err := NewClient("https://identity.example.invalid", "synthetic-read-token", "project", &http.Client{Transport: transport}); err == nil {
			t.Fatal("unverified directory transport accepted")
		}
	}
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, validResponse) }))
	defer server.Close()
	provided := server.Client()
	client, err := NewClient(server.URL, "synthetic-read-token", "project", provided)
	if err != nil {
		t.Fatal(err)
	}
	provided.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	isolated, ok := client.http.Transport.(*http.Transport)
	if !ok || isolated == provided.Transport || isolated.TLSClientConfig.InsecureSkipVerify || isolated.MaxResponseHeaderBytes > 16<<10 || isolated.MaxResponseHeaderBytes == 0 {
		t.Fatal("caller can change directory TLS verification or header limit")
	}
	if _, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20}); err != nil {
		t.Fatal(err)
	}
}

func TestListUsesProjectGrantOrganizationNotHomeOrganization(t *testing.T) {
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/zitadel.authorization.v2.AuthorizationService/ListAuthorizations" || r.Header.Get("Authorization") != "Bearer synthetic-read-token" || r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Filters    []map[string]any `json:"filters"`
			Pagination struct {
				Limit, Offset int
				Asc           bool
			} `json:"pagination"`
			Sorting string `json:"sortingColumn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		filters, _ := json.Marshal(body.Filters)
		if string(filters) != `[{"organizationId":{"id":"effective-b"}},{"projectId":{"id":"project"}}]` || body.Pagination.Limit != 20 || body.Pagination.Offset != 0 || !body.Pagination.Asc || body.Sorting != "AUTHORIZATION_FIELD_NAME_ID" {
			t.Errorf("wrong scope/query: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, validResponse)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].UserID != "user-a" || page.Items[0].OrganizationID != "effective-b" || page.Items[0].DisplayName != "Member A" || page.Items[0].State != "active" {
		t.Fatalf("wrong projection: %+v", page)
	}
}

func TestListFailsClosedOnInvalidProviderData(t *testing.T) {
	for name, response := range map[string]string{
		"foreign org":     strings.Replace(validResponse, `"id":"effective-b"`, `"id":"foreign"`, 1),
		"foreign project": strings.Replace(validResponse, `"id":"project"`, `"id":"foreign"`, 1),
		"null result":     `{"pagination":null,"authorizations":[]}`,
		"null items":      `{"pagination":{"totalResult":0},"authorizations":null}`,
		"wrong count":     strings.Replace(validResponse, `"totalResult":"1"`, `"totalResult":"0"`, 1),
		"trailing json":   validResponse + `{}`,
		"too large":       strings.Repeat(" ", 1024*1024+1),
		"unknown state":   strings.Replace(validResponse, "STATE_ACTIVE", "STATE_UNKNOWN", 1),
		"missing user":    strings.Replace(validResponse, `"id":"user-a"`, `"id":""`, 1),
		"duplicate role":  strings.Replace(validResponse, `{"key":"listingkit_viewer"}`, `{"key":"listingkit_viewer"},{"key":"listingkit_viewer"}`, 1),
		"invalid date":    strings.Replace(validResponse, `"changeDate":"2026-09-12T00:00:00Z"`, `"changeDate":"not-a-date"`, 1),
		"oversized count": strings.Replace(validResponse, `"totalResult":"1"`, `"totalResult":"18446744073709551616"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
			defer server.Close()
			client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
			page, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20})
			if err == nil || len(page.Items) != 0 {
				t.Fatalf("invalid data accepted: %+v %v", page, err)
			}
		})
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cancelAfterBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelAfterBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func TestListDoesNotReturnDataAfterCancellationWhileReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := NewClient("https://identity.example.invalid", "synthetic-read-token", "project", nil)
	if err != nil {
		t.Fatal(err)
	}
	listCalls := 0
	// Inject only after the verified production constructor, to cancel exactly
	// while consuming a directory body rather than failing the permission probe.
	client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/permissions/zitadel/me/_search") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":["user.grant.read"]}`)), Header: make(http.Header), Request: r}, nil
		}
		listCalls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(cancelAfterBody{Reader: strings.NewReader(validResponse), cancel: cancel}), Header: make(http.Header), Request: r}, nil
	})
	page, err := client.List(ctx, "effective-b", domain.PageRequest{Limit: 20})
	if err == nil || len(page.Items) != 0 || listCalls != 1 || ctx.Err() == nil {
		t.Fatalf("canceled read published data: %+v %v", page, err)
	}
}

func TestListAcceptsOfficialProtoJSONEmptyDefaults(t *testing.T) {
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"pagination":{}}`) }))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	page, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20})
	if err != nil || page.Total != 0 || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty not decoded: %+v %v", page, err)
	}
}

func TestListDoesNotFollowCredentialRedirects(t *testing.T) {
	redirectCalls := 0
	target := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls++ }))
	defer target.Close()
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	_, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20})
	if err == nil || redirectCalls != 0 {
		t.Fatalf("redirect followed: calls=%d err=%v", redirectCalls, err)
	}
}

func TestListBoundsAndCancellationDoNotReachProvider(t *testing.T) {
	calls := 0
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, validResponse) }))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	for _, page := range []domain.PageRequest{{Limit: 0}, {Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, Offset: 10001}} {
		if _, err := client.List(context.Background(), "effective-b", page); err == nil {
			t.Errorf("invalid page accepted: %+v", page)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.List(ctx, "effective-b", domain.PageRequest{Limit: 20}); err == nil {
		t.Error("canceled request accepted")
	}
	if calls != 0 {
		t.Fatalf("invalid requests reached provider: %d", calls)
	}
}

func TestListProviderFailureDoesNotExposeBody(t *testing.T) {
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"private credential detail"}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	_, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20})
	if err != domain.ErrUnavailable {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestReadUsesExactAssignmentFilterAndRejectsMismatch(t *testing.T) {
	response := validResponse
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		filters, _ := json.Marshal(body["filters"])
		if !strings.Contains(string(filters), `"authorizationIds":{"ids":["grant-b"]}`) {
			t.Errorf("missing exact filter: %s", filters)
		}
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	member, err := client.Read(context.Background(), "effective-b", "grant-b")
	if err != nil || member.ID != "grant-b" {
		t.Fatalf("detail: %+v %v", member, err)
	}
	response = strings.Replace(validResponse, "grant-b", "foreign-id", 1)
	if _, err := client.Read(context.Background(), "effective-b", "grant-b"); err != domain.ErrInvalidResponse {
		t.Fatalf("wrong target accepted: %v", err)
	}
	response = `{"pagination":{}}`
	if _, err := client.Read(context.Background(), "effective-b", "grant-b"); err != domain.ErrNotFound {
		t.Fatalf("removed target: %v", err)
	}
}
