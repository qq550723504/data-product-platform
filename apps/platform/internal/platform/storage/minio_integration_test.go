package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The orchestrator creates fresh synthetic credentials in a private file, not
// process arguments, report artifacts or the engine container environment.
func TestIngestionStorageIsolation(t *testing.T) {
	path := os.Getenv("TEST_INGESTION_STORAGE_CONFIG")
	if path == "" {
		t.Skip("TEST_INGESTION_STORAGE_CONFIG is not set")
	}
	var cfg struct{ Endpoint, AccessKey, SecretKey string }
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read private test config")
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal("decode private test config")
	}
	ctx := context.Background()
	admin, err := New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, "raw", false)
	if err != nil {
		t.Fatal("initialize test store")
	}
	if err := admin.EnsureBucket(ctx); err != nil {
		t.Fatal("create test bucket:", err)
	}
	stage := "staging/" + uuid.NewString() + "/"
	core := "core/" + uuid.NewString() + "/"
	scoped := func(statements []any) *Store {
		t.Helper()
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
		creds, err := credentials.NewSTSAssumeRole("http://"+cfg.Endpoint, credentials.STSAssumeRoleOptions{AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, Policy: string(policy)})
		if err != nil {
			t.Fatal("configure scoped test identity")
		}
		client, err := minio.New(cfg.Endpoint, &minio.Options{Creds: creds, Region: "us-east-1"})
		if err != nil {
			t.Fatal("configure scoped test client")
		}
		return &Store{client: client, bucket: "raw"}
	}
	allow := func(prefix string, actions ...string) any {
		return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
	}
	engine := scoped([]any{allow(stage, "s3:PutObject", "s3:GetObject")})
	coreStore := scoped([]any{allow(stage, "s3:GetObject"), allow(core, "s3:PutObject", "s3:GetObject")})
	denied := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s unexpectedly allowed", label)
		}
		var response minio.ErrorResponse
		if !errors.As(err, &response) || response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s must be denied by the real provider/gateway with 403", label)
		}
		t.Log("DENIED:", label)
	}
	values := []byte("id,value\n1,原始值\n")
	stageURI, err := engine.PutImmutable(ctx, stage+"enterprise.csv", bytes.NewReader(values), int64(len(values)), "text/csv")
	if err != nil {
		t.Fatal("engine create staging:", err)
	}
	reader, err := coreStore.Get(ctx, stageURI)
	if err != nil {
		t.Fatal("Core read staging:", err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, values) {
		t.Fatal("staging byte fidelity")
	}
	coreURI, err := coreStore.PutImmutable(ctx, core+"enterprise.csv", bytes.NewReader(values), int64(len(values)), "text/csv")
	if err != nil {
		t.Fatal("Core create:", err)
	}
	_, err = engine.Put(ctx, core+"enterprise.csv", bytes.NewReader(values), int64(len(values)), "text/csv")
	denied("engine Core write", err)
	_, err = engine.Get(ctx, coreURI)
	denied("engine Core read", err)
	_, err = engine.Put(ctx, "staging/another-attempt/x.csv", bytes.NewReader(values), int64(len(values)), "text/csv")
	denied("engine other-attempt write", err)
	_, err = coreStore.Put(ctx, core+"enterprise.csv", bytes.NewReader([]byte("overwrite")), 9, "text/csv")
	denied("unconditional Core overwrite", err)
	_, err = coreStore.Put(ctx, core+"new.csv", bytes.NewReader(values), int64(len(values)), "text/csv")
	denied("unconditional Core create", err)
	opts := minio.PutObjectOptions{DisableMultipart: true}
	opts.SetMatchETagExcept("an-etag-that-does-not-match")
	_, err = coreStore.client.PutObject(ctx, "raw", core+"enterprise.csv", bytes.NewReader(values), int64(len(values)), opts)
	denied("non-star conditional Core overwrite", err)
	_, err = coreStore.PutImmutable(ctx, core+"enterprise.csv", bytes.NewReader([]byte("overwrite")), 9, "text/csv")
	if !errors.Is(err, ErrObjectExists) {
		t.Fatal("conditional overwrite must return ErrObjectExists:", err)
	}
	t.Log("DENIED: conditional Core overwrite")
	denied("Core delete", coreStore.client.RemoveObject(ctx, "raw", core+"enterprise.csv", minio.RemoveObjectOptions{}))
	_, err = coreStore.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: "raw", Object: core + "enterprise.csv"}, minio.CopySrcOptions{Bucket: "raw", Object: stage + "enterprise.csv"})
	denied("Core copy overwrite", err)
	denied("engine delete", engine.client.RemoveObject(ctx, "raw", stage+"enterprise.csv", minio.RemoveObjectOptions{}))
	reader, err = coreStore.Get(ctx, coreURI)
	if err != nil {
		t.Fatal("Core re-read")
	}
	got, err = io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, values) {
		t.Fatal("failed overwrites changed Core content")
	}
	// Two authenticated writers race on the provider, not a process mutex.
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content := []byte(fmt.Sprintf("writer-%d", i))
			_, err := coreStore.PutImmutable(ctx, core+"race", bytes.NewReader(content), int64(len(content)), "text/plain")
			outcomes <- err
		}(i)
	}
	wg.Wait()
	close(outcomes)
	won, lost := 0, 0
	for err := range outcomes {
		if err == nil {
			won++
		} else if errors.Is(err, ErrObjectExists) {
			lost++
		} else {
			t.Fatal("conditional race:", err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("conditional race winners=%d conflicts=%d", won, lost)
	}
	reader, err = coreStore.Get(ctx, "s3://raw/"+core+"race")
	if err != nil {
		t.Fatal("read conditional race winner")
	}
	got, err = io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || (!bytes.Equal(got, []byte("writer-0")) && !bytes.Equal(got, []byte("writer-1"))) {
		t.Fatal("conditional race did not preserve one complete payload")
	}
	// Exercise the official mitigation with a query credential and no Authorization.
	req, _ := http.NewRequest(http.MethodPut, "http://"+cfg.Endpoint+"/raw/"+core+"bypass?X-Amz-Credential="+cfg.AccessKey+"%2F20261011%2Fus-east-1%2Fs3%2Faws4_request", bytes.NewReader(values))
	req.Header.Set("X-Amz-Content-Sha256", "STREAMING-UNSIGNED-PAYLOAD-TRAILER")
	req.Header.Set("If-None-Match", "*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("mitigation probe")
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal("unsigned-trailer mitigation was bypassed")
	}
}
