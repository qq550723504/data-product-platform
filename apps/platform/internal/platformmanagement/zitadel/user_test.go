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

const nativeUser = `{"user":{"userId":"native-user","username":"member","state":"USER_STATE_ACTIVE","details":{"resourceOwner":"org","creationDate":"2026-10-24T00:00:00Z","changeDate":"2026-10-24T00:00:00Z"},"human":{"profile":{"displayName":"Directory person"},"email":{"email":"PRIVATE_EMAIL_MUST_NOT_PROJECT"},"phone":{"phone":"PRIVATE_PHONE_MUST_NOT_PROJECT"}}}}`

func TestReadUserKeepsNativeStateAndPrivateFieldsOut(t *testing.T) {
	server := authorizedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v2/users/native-user" || r.Header.Get("x-zitadel-orgid") != "org" || r.ContentLength != 0 {
			t.Error("unscoped native user request")
		}
		fmt.Fprint(w, nativeUser)
	}))
	defer server.Close()
	c, e := NewClient(server.URL, "read", "project", server.Client())
	if e != nil {
		t.Fatal(e)
	}
	profile, e := c.ReadUser(context.Background(), "org", "native-user")
	if e != nil || profile.ID != "native-user" || profile.AccountState != "active" || profile.Username != "member" {
		t.Fatal("wrong account facts", e)
	}
	projected, _ := json.Marshal(profile)
	if strings.Contains(string(projected), "PRIVATE_") {
		t.Fatal("private native account fields projected")
	}
}
func TestReadUserRejectsForeignMachineUnknownAndRevokedSources(t *testing.T) {
	for _, fault := range []string{"foreign-owner", "foreign-id", "machine", "unknown-state", "invalid-date", "not-found", "permission-revoked"} {
		t.Run(fault, func(t *testing.T) {
			probes := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/permissions/") {
					probes++
					if fault == "permission-revoked" && probes > 1 {
						fmt.Fprint(w, `{"result":[]}`)
					} else {
						fmt.Fprint(w, `{"result":["user.read"]}`)
					}
					return
				}
				if fault == "not-found" {
					w.WriteHeader(404)
					return
				}
				body := nativeUser
				switch fault {
				case "foreign-owner":
					body = strings.Replace(body, `"resourceOwner":"org"`, `"resourceOwner":"foreign"`, 1)
				case "foreign-id":
					body = strings.Replace(body, `"userId":"native-user"`, `"userId":"other"`, 1)
				case "machine":
					body = strings.Replace(body, `"human":`, `"machine":{},"human":`, 1)
				case "unknown-state":
					body = strings.Replace(body, "USER_STATE_ACTIVE", "USER_STATE_UNSPECIFIED", 1)
				case "invalid-date":
					body = strings.Replace(body, `"changeDate":"2026-10-24T00:00:00Z"`, `"changeDate":"bad"`, 1)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, e := NewClient(server.URL, "read", "project", server.Client())
			if e != nil {
				t.Fatal(e)
			}
			profile, e := c.ReadUser(context.Background(), "org", "native-user")
			if e == nil || profile.ID != "" {
				t.Fatal("invalid or revoked account published")
			}
		})
	}
}
