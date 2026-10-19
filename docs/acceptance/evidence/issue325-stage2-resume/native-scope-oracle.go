// Read-only acceptance oracle; never part of the normal trial controller.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func read(path string, v any) {
	b, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(b, v) != nil {
		panic("private oracle input unavailable")
	}
}

func main() {
	if len(os.Args) != 2 {
		panic("registry required")
	}
	var s struct{ Private, ActiveProfile, WorkspaceID string }
	read(os.Args[1], &s)
	var p struct{ CoreCredentialFile, StorageEndpoint string }
	read(s.ActiveProfile, &p)
	var id struct{ AccessKey, SecretKey, SessionToken string }
	read(p.CoreCredentialFile, &id)
	var facts struct {
		Objects []struct{ URI, SHA256 string }
	}
	read(filepath.Join(filepath.Dir(os.Args[1]), "before-stop-facts.json"), &facts)
	client, err := minio.New(p.StorageEndpoint, &minio.Options{Creds: credentials.NewStaticV4(id.AccessKey, id.SecretKey, id.SessionToken), Secure: false, Region: "us-east-1"})
	if err != nil || id.SessionToken == "" || len(facts.Objects) != 3 {
		panic("scoped native oracle context invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, expected := range facts.Objects {
		key := strings.TrimPrefix(expected.URI, "s3://raw/")
		object, err := client.GetObject(ctx, "raw", key, minio.GetObjectOptions{})
		if err != nil {
			panic("fresh session read failed")
		}
		data, err := io.ReadAll(object)
		_ = object.Close()
		h := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(h[:]) != expected.SHA256 {
			panic("fresh session hash mismatch")
		}
	}
	_, err = client.StatObject(ctx, "raw", "core/00000000-0000-0000-0000-000000000000/resume-scope-probe", minio.StatObjectOptions{})
	denial := minio.ToErrorResponse(err)
	if err == nil || denial.Code != "AccessDenied" || denial.StatusCode != 403 {
		panic("other-workspace scope was not denied")
	}
	_, _ = fmt.Fprintln(os.Stdout, `{"status":"PASS","freshSessionRawObjects":3,"allHashesMatch":true,"foreignWorkspaceStatus":403,"foreignWorkspaceCode":"AccessDenied","mutations":0}`)
}
