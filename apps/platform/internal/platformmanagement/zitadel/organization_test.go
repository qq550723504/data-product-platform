package zitadel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const nativeOrganization = `{"details":{"totalResult":"1"},"result":[{"id":"org","name":"Directory Org","state":"ORGANIZATION_STATE_ACTIVE","details":{"creationDate":"2026-10-24T00:00:00Z","changeDate":"2026-10-25T00:00:00Z","resourceOwner":"identity-instance"},"primaryDomain":"PRIVATE_DOMAIN","metadata":{"private":"PRIVATE_METADATA"}}]}`

func TestOrganizationExactV2ScopeAndMinimalProjection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/permissions/") {
			fmt.Fprint(w, `{"result":["org.read"]}`)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/v2/organizations/_search" || r.Header.Get("x-zitadel-orgid") != "org" {
			t.Error("wrong org query scope")
		}
		var q organizationReadRequest
		if json.NewDecoder(r.Body).Decode(&q) != nil || q != organizationRequest("org") {
			t.Error("not an exact single-organization query")
		}
		fmt.Fprint(w, nativeOrganization)
	}))
	defer server.Close()
	c, e := NewClient(server.URL, "read", "project", server.Client())
	if e != nil {
		t.Fatal(e)
	}
	o, e := c.ReadOrganization(context.Background(), "org")
	if e != nil || o.ID != "org" || o.Name == nil || *o.Name != "Directory Org" || o.State != "active" {
		t.Fatal("invalid organization projection", e)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "resourceOwner") {
		t.Fatal("unneeded native facts projected")
	}
	for _, body := range []any{map[string]any{}, organizationRequest("foreign")} {
		if _, e := c.requestJSON(context.Background(), "POST", "/v2/organizations/_search", body, false, "org"); e == nil {
			t.Fatal("broad or foreign query allowed")
		}
	}
}
func TestOrganizationRejectsForeignIncompleteAndRevokedSources(t *testing.T) {
	for _, fault := range []string{"foreign", "extra", "missing-total", "wrong-total", "unknown-state", "bad-date", "blank-name", "not-found", "permission-revoked", "suffixed-permission", "unavailable", "sparse"} {
		t.Run(fault, func(t *testing.T) {
			probes := 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/permissions/") {
					probes++
					if fault == "suffixed-permission" {
						fmt.Fprint(w, `{"result":["org.read:foreign"]}`)
					} else if fault == "permission-revoked" && probes > 1 {
						fmt.Fprint(w, `{"result":[]}`)
					} else {
						fmt.Fprint(w, `{"result":["org.read"]}`)
					}
					return
				}
				body := nativeOrganization
				switch fault {
				case "foreign":
					body = strings.Replace(body, `"id":"org"`, `"id":"foreign"`, 1)
				case "extra":
					body = `{"details":{"totalResult":"2"},"result":[{"id":"org"},{"id":"foreign"}]}`
				case "missing-total":
					body = strings.Replace(body, `"totalResult":"1"`, `"timestamp":"2026-10-25T00:00:00Z"`, 1)
				case "wrong-total":
					body = strings.Replace(body, `"totalResult":"1"`, `"totalResult":"0"`, 1)
				case "unknown-state":
					body = strings.Replace(body, "ORGANIZATION_STATE_ACTIVE", "ORGANIZATION_STATE_UNSPECIFIED", 1)
				case "bad-date":
					body = strings.Replace(body, "2026-10-24T00:00:00Z", "bad", 1)
				case "blank-name":
					body = strings.Replace(body, "Directory Org", "   ", 1)
				case "not-found":
					body = `{"details":{"totalResult":"0"},"result":[]}`
				case "unavailable":
					w.WriteHeader(503)
					return
				case "sparse":
					body = `{"details":{"totalResult":1},"result":[{"id":"org","state":"ORGANIZATION_STATE_INACTIVE"}]}`
				}
				fmt.Fprint(w, body)
			}))
			defer s.Close()
			c, e := NewClient(s.URL, "read", "project", s.Client())
			if e != nil {
				t.Fatal(e)
			}
			o, e := c.ReadOrganization(context.Background(), "org")
			if fault == "sparse" {
				if e != nil || o.Name != nil || o.CreatedAt != nil || o.ChangedAt != nil || o.State != "inactive" {
					t.Fatal("invented missing fields", e)
				}
			} else if e == nil || o.ID != "" {
				t.Fatal("invalid organization returned", fault)
			}
		})
	}
}
