package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

type Profile struct {
	Version   string     `json:"version"`
	Mode      string     `json:"mode"`
	Listen    string     `json:"listen"`
	TLS       ServerTLS  `json:"tls"`
	Identity  Identity   `json:"identity"`
	Database  Database   `json:"database"`
	Audit     *Audit     `json:"audit"`
	Directory *Directory `json:"directory"`
}
type ServerTLS struct {
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`
}
type Identity struct {
	Source                string   `json:"source"`
	Issuer                string   `json:"issuer"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	IntrospectionEndpoint string   `json:"introspectionEndpoint"`
	ClientID              string   `json:"clientId"`
	ClientSecretFile      string   `json:"clientSecretFile"`
	Audience              string   `json:"audience"`
	RequiredScope         string   `json:"requiredScope"`
	AllowedClientIDs      []string `json:"allowedClientIds"`
	CAFile                string   `json:"caFile"`
}
type Database struct {
	URLFile string `json:"urlFile"`
	CAFile  string `json:"caFile"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}
type Audit struct {
	Source     string      `json:"source"`
	Workspaces []uuid.UUID `json:"workspaces"`
}

// Private, startup-only adapter configuration. It never creates caller grants.
type Directory struct {
	Source     string               `json:"source"`
	Provider   string               `json:"provider"`
	Workspaces []DirectoryWorkspace `json:"workspaces"`
}
type DirectoryWorkspace struct {
	WorkspaceID    uuid.UUID         `json:"workspaceId"`
	Origin         string            `json:"origin"`
	CAFile         string            `json:"caFile"`
	CredentialFile string            `json:"credentialFile"`
	OrganizationID string            `json:"organizationId"`
	ProjectID      string            `json:"projectId"`
	Roles          []management.Role `json:"roles"`
}

func (Directory) String() string              { return "Directory configuration (private)" }
func (d Directory) GoString() string          { return d.String() }
func (DirectoryWorkspace) String() string     { return "Directory workspace configuration (private)" }
func (d DirectoryWorkspace) GoString() string { return d.String() }

func (Profile) String() string     { return "Management profile (private)" }
func (p Profile) GoString() string { return p.String() }

func readPrivate(file string, max int64) ([]byte, error) {
	if !filepath.IsAbs(file) {
		return nil, ErrConfiguration
	}
	f, e := os.Open(file)
	if e != nil {
		return nil, ErrConfiguration
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, ErrConfiguration
	}
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || len(b) == 0 || int64(len(b)) > max {
		return nil, ErrConfiguration
	}
	return b, nil
}

func LoadProfile(file string) (Profile, error) {
	var p Profile
	b, e := readPrivate(file, 32768)
	if e != nil || strictProfile(b, &p) != nil || p.Validate() != nil {
		return Profile{}, ErrConfiguration
	}
	return p, nil
}

// Reuses the source-admission token-walk pattern around the standard JSON decoder;
// this adds duplicate/case-sensitive closed-key checks, not a JSON implementation.
func strictProfile(b []byte, p *Profile) error {
	if !utf8.Valid(b) {
		return ErrConfiguration
	}
	allowed := strings.Fields("version mode listen tls identity database audit directory certFile keyFile source issuer tokenEndpoint introspectionEndpoint clientId clientSecretFile audience requiredScope allowedClientIds caFile urlFile name role workspaces provider workspaceId origin credentialFile organizationId projectId roles id permissions")
	keys := map[string]bool{}
	for _, k := range allowed {
		keys[k] = true
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 8 {
			return ErrConfiguration
		}
		t, e := d.Token()
		if e != nil {
			return ErrConfiguration
		}
		if delim, ok := t.(json.Delim); ok {
			if delim == '{' {
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return ErrConfiguration
					}
					key, ok := k.(string)
					if !ok || !keys[key] || seen[key] {
						return ErrConfiguration
					}
					seen[key] = true
					if walk(depth+1) != nil {
						return ErrConfiguration
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return ErrConfiguration
				}
			} else if delim == '[' {
				for d.More() {
					if walk(depth+1) != nil {
						return ErrConfiguration
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return ErrConfiguration
				}
			} else {
				return ErrConfiguration
			}
		}
		return nil
	}
	if walk(0) != nil {
		return ErrConfiguration
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrConfiguration
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(p) != nil {
		return ErrConfiguration
	}
	return nil
}

func (p Profile) Validate() error {
	host, port, e := net.SplitHostPort(p.Listen)
	n, pe := strconv.Atoi(port)
	if p.Version != "platform-management-query.v1" || p.Mode != "QUERY_ONLY" || e != nil || pe != nil || n < 1024 || n > 65535 || strconv.Itoa(n) != port || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return ErrConfiguration
	}
	for _, f := range []string{p.TLS.CertFile, p.TLS.KeyFile, p.Identity.ClientSecretFile, p.Identity.CAFile, p.Database.URLFile, p.Database.CAFile} {
		if !filepath.IsAbs(f) {
			return ErrConfiguration
		}
	}
	if p.Identity.Source != "CONFIGURED" || p.Identity.ClientID == "" || p.Identity.Audience == "" || p.Identity.RequiredScope != "platform-management" || len(p.Identity.AllowedClientIDs) == 0 || p.Database.Name == "" || p.Database.Role == "" {
		return ErrConfiguration
	}
	for _, raw := range []string{p.Identity.Issuer, p.Identity.TokenEndpoint, p.Identity.IntrospectionEndpoint} {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return ErrConfiguration
		}
	}
	seenClient := map[string]bool{}
	for _, id := range p.Identity.AllowedClientIDs {
		if strings.TrimSpace(id) == "" || seenClient[id] {
			return ErrConfiguration
		}
		seenClient[id] = true
	}
	if p.Audit != nil {
		if p.Audit.Source != "CONFIGURED" || len(p.Audit.Workspaces) == 0 {
			return ErrConfiguration
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range p.Audit.Workspaces {
			if id == uuid.Nil || seen[id] {
				return ErrConfiguration
			}
			seen[id] = true
		}
	}
	if p.Directory != nil {
		if p.Directory.Source != "CONFIGURED" || p.Directory.Provider != "ZITADEL" || len(p.Directory.Workspaces) == 0 || len(p.Directory.Workspaces) > 32 {
			return ErrConfiguration
		}
		seen := map[uuid.UUID]bool{}
		for _, b := range p.Directory.Workspaces {
			u, e := url.Parse(b.Origin)
			if b.WorkspaceID == uuid.Nil || seen[b.WorkspaceID] || !management.IsIdentifier(b.OrganizationID) || !management.IsIdentifier(b.ProjectID) || !filepath.IsAbs(b.CAFile) || !filepath.IsAbs(b.CredentialFile) || e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
				return ErrConfiguration
			}
			if _, e := management.NewCatalog(b.Roles); e != nil {
				return ErrConfiguration
			}
			seen[b.WorkspaceID] = true
		}
	}
	return nil
}

// Prevent accidental configuration diagnostics from dumping nested private inputs.
var _ fmt.Stringer = Profile{}
