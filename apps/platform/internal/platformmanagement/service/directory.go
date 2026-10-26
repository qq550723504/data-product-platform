package service

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/zitadel"
)

// Construct existing adapters without making source requests. Health/readiness
// does not claim directory availability or real-source admission.
func directoryBindings(config *Directory) (map[uuid.UUID]management.Binding, func(), error) {
	bindings := map[uuid.UUID]management.Binding{}
	var clients []*zitadel.Client
	close := func() {
		for _, c := range clients {
			c.CloseIdleConnections()
		}
	}
	if config == nil {
		return bindings, close, nil
	}
	for _, b := range config.Workspaces {
		ca, e := roots(b.CAFile)
		if e != nil {
			close()
			return nil, nil, ErrConfiguration
		}
		raw, e := readPrivate(b.CredentialFile, 16384)
		credential := strings.TrimSpace(string(raw))
		if e != nil || credential == "" || strings.ContainsFunc(credential, func(r rune) bool { return r < 33 || r > 126 }) {
			close()
			return nil, nil, ErrConfiguration
		}
		catalog, e := management.NewCatalog(b.Roles)
		if e != nil {
			close()
			return nil, nil, ErrConfiguration
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: ca, MinVersion: tls.VersionTLS12}}, Timeout: 10 * time.Second}
		native, e := zitadel.NewClient(b.Origin, credential, b.ProjectID, client)
		if e != nil {
			close()
			return nil, nil, ErrConfiguration
		}
		clients = append(clients, native)
		// User profiles and native-user -> platform identity links require their
		// own approved source contract. Do not enable every method on this client.
		bindings[b.WorkspaceID] = management.Binding{OrganizationID: b.OrganizationID, ProjectID: b.ProjectID, Source: config.Source, Directory: native, Organizations: native, Catalog: catalog}
	}
	return bindings, close, nil
}
