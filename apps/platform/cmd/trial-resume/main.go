// trial-resume refreshes the existing trial's Core STS identity only.
// It has no database, migration, bootstrap, user/policy management or seed path.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

type config struct {
	Private, Workspace, RootKey, RootSecret string
	Profile                                 struct{ StorageEndpoint, Bucket string }
}

func policy(c config) ([]byte, error) {
	ws, err := uuid.Parse(c.Workspace)
	host, port, hostErr := net.SplitHostPort(c.Profile.StorageEndpoint)
	if err != nil || ws == uuid.Nil || hostErr != nil || host != "127.0.0.1" || port == "" || c.Profile.Bucket != "raw" ||
		!filepath.IsAbs(c.Private) || !strings.HasPrefix(filepath.Base(c.Private), "dpp-trial-") {
		return nil, fmt.Errorf("same isolated trial configuration required")
	}
	allow := func(prefix string, actions ...string) any {
		return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
	}
	return json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{
		allow("staging/"+ws.String()+"/", "s3:GetObject"), allow("core/"+ws.String()+"/", "s3:GetObject", "s3:PutObject"),
	}})
}

func renew(c config, output string) (any, error) {
	p, err := policy(c)
	if err != nil {
		return nil, err
	}
	private, err := filepath.EvalSymlinks(c.Private)
	if err != nil {
		return nil, fmt.Errorf("private trial directory required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return nil, fmt.Errorf("private resume directory required")
	}
	rel, err := filepath.Rel(private, parent)
	parts := strings.Split(rel, string(filepath.Separator))
	if err != nil || len(parts) != 2 || parts[0] != "resume" || filepath.Base(output) != "core-identity.json" {
		return nil, fmt.Errorf("new credential must stay in private resume attempt")
	}
	if id, err := uuid.Parse(parts[1]); err != nil || id == uuid.Nil {
		return nil, fmt.Errorf("resume attempt required")
	}
	// Reserve the new file before any invocation; same-attempt replay cannot
	// issue another identity or overwrite an old credential.
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("new credential output already exists or unavailable")
	}
	defer f.Close()
	creds, err := credentials.NewSTSAssumeRole("http://"+c.Profile.StorageEndpoint, credentials.STSAssumeRoleOptions{
		AccessKey: c.RootKey, SecretKey: c.RootSecret, Policy: string(p), DurationSeconds: 3600,
	})
	if err != nil {
		return nil, fmt.Errorf("native STS configuration failed")
	}
	v, err := creds.GetWithContext(&credentials.CredContext{Client: &http.Client{
		Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("STS redirect refused") },
	}})
	if err != nil {
		return nil, fmt.Errorf("native STS outcome UNKNOWN; retain resume attempt")
	}
	if v.AccessKeyID == "" || v.SecretAccessKey == "" || v.SessionToken == "" || !v.Expiration.After(time.Now().Add(time.Minute)) {
		return nil, fmt.Errorf("finite native STS identity required")
	}
	identity := struct{ AccessKey, SecretKey, SessionToken string }{v.AccessKeyID, v.SecretAccessKey, v.SessionToken}
	if json.NewEncoder(f).Encode(identity) != nil || f.Sync() != nil {
		return nil, fmt.Errorf("credential persistence UNKNOWN")
	}
	h := sha256.Sum256(p)
	return map[string]any{"expiresAt": v.Expiration.UTC(), "policySha256": hex.EncodeToString(h[:]), "nativeSTSInvocations": 1}, nil
}

func verifyCore(c config, identityPath, receiptPath string) (any, error) {
	if _, err := policy(c); err != nil {
		return nil, err
	}
	var identity struct{ AccessKey, SecretKey, SessionToken string }
	b, err := os.ReadFile(identityPath)
	if err != nil || json.Unmarshal(b, &identity) != nil || identity.SessionToken == "" {
		return nil, fmt.Errorf("private session identity required")
	}
	var receipt struct {
		WorkspaceID string
		Objects     []struct {
			URI, SHA256 string
			Bytes       int64
		}
	}
	b, err = os.ReadFile(receiptPath)
	if err != nil || json.Unmarshal(b, &receipt) != nil || receipt.WorkspaceID != c.Workspace || len(receipt.Objects) == 0 {
		return nil, fmt.Errorf("same-workspace receipt required")
	}
	for _, object := range receipt.Objects {
		u, err := url.Parse(object.URI)
		if err != nil || u.Scheme != "s3" || u.Host != "raw" || u.RawQuery != "" || u.Fragment != "" || path.Clean(u.Path) != u.Path || !strings.HasPrefix(u.Path, "/core/"+c.Workspace+"/") || object.Bytes <= 0 || len(object.SHA256) != 64 {
			return nil, fmt.Errorf("frozen same-workspace object required")
		}
	}
	store, err := storage.NewWithCredentials(c.Profile.StorageEndpoint, "raw", false, credentials.NewStaticV4(identity.AccessKey, identity.SecretKey, identity.SessionToken))
	if err != nil {
		return nil, fmt.Errorf("native scoped read client failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, object := range receipt.Objects {
		reader, err := store.Get(ctx, object.URI)
		if err != nil {
			return nil, fmt.Errorf("fresh Core identity cannot read retained objects")
		}
		data, err := io.ReadAll(io.LimitReader(reader, 128<<20))
		_ = reader.Close()
		h := sha256.Sum256(data)
		if err != nil || int64(len(data)) != object.Bytes || hex.EncodeToString(h[:]) != object.SHA256 {
			return nil, fmt.Errorf("fresh Core identity object hash mismatch")
		}
	}
	return map[string]any{"status": "PASS", "verifiedObjects": len(receipt.Objects), "scope": "original workspace Core objects"}, nil
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "renew-core" && os.Args[1] != "verify-core") {
		return fmt.Errorf("renew-core|verify-core required; no bootstrap operations")
	}
	b, err := os.ReadFile(os.Getenv("DPP_TRIAL_CONFIG"))
	if err != nil {
		return fmt.Errorf("private trial config required")
	}
	var c config
	if json.Unmarshal(b, &c) != nil {
		return fmt.Errorf("invalid private trial config")
	}
	var v any
	if os.Args[1] == "renew-core" {
		v, err = renew(c, os.Getenv("DPP_TRIAL_RENEW_OUTPUT"))
	} else {
		v, err = verifyCore(c, os.Getenv("DPP_TRIAL_RENEW_OUTPUT"), os.Getenv("DPP_TRIAL_VERIFY_RECEIPT"))
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(v)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
