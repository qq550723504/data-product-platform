package pilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

// WorkspaceObjectProfile is private, server-owned composition. It contains
// credential references, not credential secrets or user-supplied scope claims.
type WorkspaceObjectProfile struct {
	Scope          lc.ProtectedScope
	Endpoint       string
	Secure         bool
	CredentialFile string
}

func BuildWorkspaceObjects(pool *pgxpool.Pool, path string) (*lc.ProtectedObjects, error) {
	var profile WorkspaceObjectProfile
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &profile) != nil || profile.Scope.Validate() != nil || strings.TrimSpace(profile.Endpoint) == "" {
		return nil, fmt.Errorf("workspace object profile unavailable")
	}
	var identity struct{ AccessKey, SecretKey, SessionToken string }
	b, err = os.ReadFile(profile.CredentialFile)
	if err != nil || json.Unmarshal(b, &identity) != nil || identity.AccessKey == "" || identity.SecretKey == "" || identity.SessionToken == "" {
		return nil, fmt.Errorf("finite workspace object identity unavailable")
	}
	service, err := lc.NewProtectedObjects(pool, profile.Scope)
	if err != nil {
		return nil, err
	}
	if err = service.CheckWorkspace(context.Background(), profile.Scope.WorkspaceID); err != nil {
		return nil, err
	}
	adapter, err := storage.NewProtectedAdapter(profile.Endpoint, profile.Scope.Bucket, profile.Secure, credentials.NewStaticV4(identity.AccessKey, identity.SecretKey, identity.SessionToken), service)
	if err != nil {
		return nil, err
	}
	if err = service.BindPort(adapter); err != nil {
		return nil, err
	}
	return service, nil
}
