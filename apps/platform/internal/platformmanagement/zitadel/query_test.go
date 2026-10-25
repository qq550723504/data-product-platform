// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package zitadel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

var directoryQueryRole = "role-one"

func queryRow(index int) map[string]any {
	return map[string]any{
		"id": fmt.Sprintf("grant-%03d", index), "creationDate": "2026-09-12T00:00:00Z", "changeDate": "2026-09-12T00:00:00Z",
		"project": map[string]string{"id": "project"}, "organization": map[string]string{"id": "effective-b"},
		"user":  map[string]string{"id": fmt.Sprintf("user-%03d", index), "organizationId": "home-a", "displayName": "ordinary", "preferredLoginName": fmt.Sprintf("member-%03d@example.invalid", index)},
		"state": "STATE_INACTIVE", "roles": []map[string]string{{"key": "role-two"}, {"key": directoryQueryRole}},
	}
}

func TestDirectoryQuerySearchesCompleteFilteredDirectoryBeforePaging(t *testing.T) {
	rows := make([]map[string]any, 125)
	for i := range rows {
		rows[i] = queryRow(i)
	}
	rows[110]["user"].(map[string]string)["displayName"] = "TARGET 目标成员"
	rows[111]["user"].(map[string]string)["preferredLoginName"] = "target@other.invalid"
	rows[112]["user"].(map[string]string)["displayName"] = "ΟΣ"
	rows[113]["user"].(map[string]string)["preferredLoginName"] = "ſ@example.invalid"
	for _, tc := range []struct {
		search        string
		offset, total int
		id            string
	}{
		{"target", 0, 2, "grant-110"}, {"TaRgEt", 1, 2, "grant-111"}, {"目标", 0, 1, "grant-110"},
		{"not-present", 0, 0, ""}, {"target", 20, 2, ""},
		{"ος", 0, 1, "grant-112"}, {"S", 0, 1, "grant-113"},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.search, tc.offset), func(t *testing.T) {
			reads := 0
			server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Pagination struct {
						Limit, Offset int
						Asc           bool
					}
					SortingColumn string
					Filters       []map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					return
				}
				filters, _ := json.Marshal(input.Filters)
				if string(filters) != fmt.Sprintf(`[{"organizationId":{"id":"effective-b"}},{"projectId":{"id":"project"}},{"roleKey":{"key":"%s"}},{"state":{"state":"STATE_INACTIVE"}}]`, directoryQueryRole) {
					t.Errorf("wrong scope/filter: %s", filters)
				}
				if input.Pagination.Limit != 100 || input.Pagination.Offset != reads*100 || !input.Pagination.Asc || input.SortingColumn != "AUTHORIZATION_FIELD_NAME_ID" {
					t.Errorf("wrong traversal: %+v", input)
				}
				reads++
				end := min(input.Pagination.Offset+input.Pagination.Limit, len(rows))
				start := min(input.Pagination.Offset, end)
				_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]string{"totalResult": "125"}, "authorizations": rows[start:end]})
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
			result, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 1, Offset: tc.offset, Filter: domain.ListFilter{Search: tc.search, Role: directoryQueryRole, State: "inactive"}})
			if err != nil || result.Total != tc.total || reads != 2 {
				t.Fatalf("result=%+v reads=%d err=%v", result, reads, err)
			}
			if tc.id == "" {
				if result.Items == nil || len(result.Items) != 0 {
					t.Fatal("not a true empty page")
				}
			} else if len(result.Items) != 1 || result.Items[0].ID != tc.id {
				t.Fatalf("wrong page: %+v", result)
			}
		})
	}
}

func TestDirectoryQueryNeverPublishesIncompleteSearch(t *testing.T) {
	for _, fault := range []string{"short-page", "changed-total", "repeated-id", "unordered-id", "provider-failure", "over-bound", "cross-scope", "wrong-role", "wrong-state", "cancel", "permission-loss"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads, probes := 0, 0
			server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if fault == "provider-failure" && reads == 2 {
					w.WriteHeader(503)
					return
				}
				if fault == "cancel" && reads == 2 {
					cancel()
					w.WriteHeader(503)
					return
				}
				total, count := 101, 100
				if reads == 2 {
					count = 1
				}
				if fault == "short-page" && reads == 1 {
					count--
				}
				if fault == "changed-total" && reads == 2 {
					total++
				}
				if fault == "over-bound" {
					total = 10001
				}
				rows := make([]map[string]any, count)
				for i := range rows {
					rows[i] = queryRow((reads-1)*100 + i)
				}
				if fault == "repeated-id" && reads == 2 {
					rows[0] = queryRow(0)
				}
				if fault == "unordered-id" && reads == 1 {
					rows[0], rows[1] = rows[1], rows[0]
				}
				if fault == "cross-scope" {
					rows[0]["organization"] = map[string]string{"id": "other"}
				}
				if fault == "wrong-role" {
					rows[0]["roles"] = []map[string]string{{"key": "listingkit_admin"}}
				}
				if fault == "wrong-state" {
					rows[0]["state"] = "STATE_ACTIVE"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]string{"totalResult": fmt.Sprint(total)}, "authorizations": rows})
			}))
			defer server.Close()
			transport := transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/permissions/zitadel/me/_search") {
					probes++
					if fault == "permission-loss" && probes == 4 {
						return &http.Response{StatusCode: 200, Body: http.NoBody, Header: make(http.Header), Request: r}, nil
					}
				}
				return server.Client().Transport.RoundTrip(r)
			})
			client, _ := NewClient(server.URL, "synthetic-read-token", "project", &http.Client{Transport: transport})
			result, err := client.List(ctx, "effective-b", domain.PageRequest{Limit: 20, Filter: domain.ListFilter{Search: "ordinary", Role: directoryQueryRole, State: "inactive"}})
			if err == nil || result.Total != 0 || len(result.Items) != 0 {
				t.Fatalf("partial success: %+v %v", result, err)
			}
		})
	}
}

func TestDirectoryQueryNativeFiltersUseOnePage(t *testing.T) {
	reads := 0
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		var input struct {
			Pagination struct{ Limit, Offset int }
			Filters    []map[string]any
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		filters, _ := json.Marshal(input.Filters)
		if input.Pagination.Limit != 20 || input.Pagination.Offset != 20 || string(filters) != fmt.Sprintf(`[{"organizationId":{"id":"effective-b"}},{"projectId":{"id":"project"}},{"roleKey":{"key":"%s"}},{"state":{"state":"STATE_INACTIVE"}}]`, directoryQueryRole) {
			t.Errorf("wrong native page: %+v", input)
		}
		rows := make([]map[string]any, 5)
		for i := range rows {
			rows[i] = queryRow(20 + i)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]string{"totalResult": "25"}, "authorizations": rows})
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", server.Client())
	page, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20, Offset: 20, Filter: domain.ListFilter{Role: directoryQueryRole, State: "inactive"}})
	if err != nil || reads != 1 || page.Total != 25 || len(page.Items) != 5 {
		t.Fatalf("page=%+v reads=%d err=%v", page, reads, err)
	}
}

func TestDirectoryQueryTenThousandBoundarySharesOneDeadline(t *testing.T) {
	reads := 0
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Pagination struct{ Offset int } }
		_ = json.NewDecoder(r.Body).Decode(&input)
		reads++
		rows := make([]map[string]any, 100)
		for i := range rows {
			index := input.Pagination.Offset + i
			rows[i] = queryRow(index)
			rows[i]["id"] = fmt.Sprintf("grant-%05d", index)
			if index == 9999 {
				rows[i]["user"].(map[string]string)["displayName"] = "last target"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"pagination": map[string]string{"totalResult": "10000"}, "authorizations": rows})
	}))
	defer server.Close()
	var deadline time.Time
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		current, ok := r.Context().Deadline()
		if !ok || time.Until(current) > 10*time.Second {
			t.Errorf("missing bounded deadline: %v", current)
		}
		if deadline.IsZero() {
			deadline = current
		} else if !deadline.Equal(current) {
			t.Errorf("per-page deadline replenished: %v != %v", current, deadline)
		}
		return server.Client().Transport.RoundTrip(r)
	})
	client, _ := NewClient(server.URL, "synthetic-read-token", "project", &http.Client{Transport: transport})
	page, err := client.List(context.Background(), "effective-b", domain.PageRequest{Limit: 20, Filter: domain.ListFilter{Search: "last target"}})
	if err != nil || reads != 100 || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "grant-09999" {
		t.Fatalf("page=%+v reads=%d err=%v", page, reads, err)
	}
}
