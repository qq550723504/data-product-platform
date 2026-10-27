package service

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func directoryProfile(t *testing.T) Profile {
	p := validProfile(t)
	p.Directory = &Directory{Source: "CONFIGURED", Provider: "ZITADEL", Workspaces: []DirectoryWorkspace{{WorkspaceID: uuid.New(), Origin: "https://directory.invalid", CAFile: filepath.Join(t.TempDir(), "ca"), CredentialFile: filepath.Join(t.TempDir(), "credential"), OrganizationID: "org", ProjectID: "project", Roles: []management.Role{{ID: "viewer", Name: "查询", Permissions: []string{"management.members.read"}}}}}}
	return p
}

func TestDirectoryProfileRejectsIncompleteAmbiguousAndExpandedScope(t *testing.T) {
	for _, name := range []string{"valid", "missing-source", "synthetic", "provider", "empty", "duplicate-workspace", "zero-workspace", "organization", "project", "relative-CA", "relative-credential", "http", "userinfo", "path", "query", "empty-query", "fragment", "escaped-path", "empty-roles", "duplicate-role", "write-permission", "unknown-permission", "duplicate-permission"} {
		t.Run(name, func(t *testing.T) {
			p := directoryProfile(t)
			d := p.Directory
			b := &d.Workspaces[0]
			switch name {
			case "missing-source":
				d.Source = ""
			case "synthetic":
				d.Source = "SYNTHETIC_FIXTURE"
			case "provider":
				d.Provider = "AUTO"
			case "empty":
				d.Workspaces = nil
			case "duplicate-workspace":
				d.Workspaces = append(d.Workspaces, *b)
			case "zero-workspace":
				b.WorkspaceID = uuid.Nil
			case "organization":
				b.OrganizationID = "../org"
			case "project":
				b.ProjectID = ""
			case "relative-CA":
				b.CAFile = "ca"
			case "relative-credential":
				b.CredentialFile = "credential"
			case "http":
				b.Origin = "http://directory.invalid"
			case "userinfo":
				b.Origin = "https://user:private@directory.invalid"
			case "path":
				b.Origin += "/api"
			case "query":
				b.Origin += "?tenant=other"
			case "empty-query":
				b.Origin += "?"
			case "fragment":
				b.Origin += "#other"
			case "escaped-path":
				b.Origin += "/%2f"
			case "empty-roles":
				b.Roles = nil
			case "duplicate-role":
				b.Roles = append(b.Roles, b.Roles[0])
			case "write-permission":
				b.Roles[0].Permissions = []string{"management.members.write"}
			case "unknown-permission":
				b.Roles[0].Permissions = []string{"PLATFORM_MANAGEMENT_READ"}
			case "duplicate-permission":
				b.Roles[0].Permissions = append(b.Roles[0].Permissions, b.Roles[0].Permissions[0])
			}
			raw, _ := json.Marshal(p)
			file := filepath.Join(t.TempDir(), "profile.json")
			if os.WriteFile(file, raw, 0600) != nil {
				t.Fatal("fixture")
			}
			_, e := LoadProfile(file)
			if (e == nil) != (name == "valid") {
				t.Fatalf("directory profile %s boundary", name)
			}
		})
	}
	for _, alias := range []string{"workspaceId", "credentialFile", "permissions", "userProfiles"} {
		p := directoryProfile(t)
		raw, _ := json.Marshal(p)
		// Both duplicate keys and case aliases must fail at their actual nesting.
		for _, duplicate := range []bool{false, true} {
			key := alias
			if !duplicate {
				key = strings.ToUpper(alias[:1]) + alias[1:]
			}
			bad := strings.Replace(string(raw), `"`+alias+`":`, `"`+key+`":null,"`+alias+`":`, 1)
			file := filepath.Join(t.TempDir(), "profile.json")
			_ = os.WriteFile(file, []byte(bad), 0600)
			if _, e := LoadProfile(file); e == nil {
				t.Fatal("nested directory ambiguity accepted")
			}
		}
	}
}

func TestDirectoryAssemblyIsLazyScopedAndDoesNotEnableUserProfiles(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer dedicated-directory-only" || r.Header.Get("x-zitadel-orgid") != "org" || r.Method != "POST" {
			t.Error("wrong private source context")
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v1/permissions/zitadel/me/_search":
			_, _ = w.Write([]byte(`{"result":["org.read"]}`))
		case "/v2/organizations/_search":
			_, _ = w.Write([]byte(`{"details":{"totalResult":"1"},"result":[{"id":"org","state":"ORGANIZATION_STATE_ACTIVE","details":{"resourceOwner":"org"}}]}`))
		default:
			t.Error("unopened source route")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	p := directoryProfile(t)
	b := &p.Directory.Workspaces[0]
	b.Origin = s.URL
	if os.WriteFile(b.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600) != nil || os.WriteFile(b.CredentialFile, []byte("dedicated-directory-only\n"), 0600) != nil {
		t.Fatal("fixture")
	}
	if p.Validate() != nil {
		t.Fatal("valid profile")
	}
	bindings, close, e := directoryBindings(p.Directory)
	if e != nil {
		t.Fatal("directory assembly")
	}
	defer close()
	if calls != 0 || len(bindings) != 1 {
		t.Fatal("assembly probed source or guessed workspace")
	}
	actual := bindings[b.WorkspaceID]
	if actual.Users != nil || actual.IdentityLinks != nil || actual.Directory == nil || actual.Organizations == nil || actual.Catalog == nil || actual.Source != "CONFIGURED" {
		t.Fatal("scope amplified")
	}
	if _, e := actual.Organizations.ReadOrganization(context.Background(), "org"); e != nil || calls != 3 {
		t.Fatal("existing organization adapter failed", e, calls)
	}
	if perms, e := actual.Catalog.Permissions([]string{"unknown"}); e != nil || len(perms) != 0 {
		t.Fatal("unknown role granted permissions")
	}
	if strings.Contains(fmt.Sprintf("%#v %v", p.Directory, *b), s.URL) {
		t.Fatal("private directory diagnostic")
	}
	// Only an explicit workspace switch reuses the same native reader. It
	// never wires the independent canonical identity mapping port.
	b.UserProfiles = true
	enabled, stop, err := directoryBindings(p.Directory)
	if err != nil {
		t.Fatal("explicit user profile assembly")
	}
	defer stop()
	if enabled[b.WorkspaceID].Users == nil || enabled[b.WorkspaceID].IdentityLinks != nil || calls != 3 {
		t.Fatal("profile activation probed source or enabled identity mapping")
	}
	for _, token := range []string{"", "bad\ncredential", "bad\tcredential"} {
		_ = os.WriteFile(b.CredentialFile, []byte(token), 0600)
		if _, _, e := directoryBindings(p.Directory); e != ErrConfiguration {
			t.Fatal("invalid private credential accepted")
		}
	}
}

func TestUserProfileSwitchIsStrictAndDefaultsClosed(t *testing.T) {
	p := directoryProfile(t)
	raw, _ := json.Marshal(p)
	for _, value := range []string{"omitted", "false", "true", "null", `"true"`, "1", "{}", "[]"} {
		t.Run(value, func(t *testing.T) {
			v := strings.Replace(string(raw), `,"userProfiles":false`, "", 1)
			if value != "omitted" {
				v = strings.Replace(v, `"workspaceId":`, `"userProfiles":`+value+`,"workspaceId":`, 1)
			}
			f := filepath.Join(t.TempDir(), "profile.json")
			if os.WriteFile(f, []byte(v), 0600) != nil {
				t.Fatal("fixture")
			}
			got, err := LoadProfile(f)
			valid := value == "omitted" || value == "false" || value == "true"
			if (err == nil) != valid {
				t.Fatal("invalid boolean boundary")
			}
			if valid && got.Directory.Workspaces[0].UserProfiles != (value == "true") {
				t.Fatal("default profile boundary")
			}
		})
	}
}
