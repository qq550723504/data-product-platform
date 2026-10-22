package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"net/http"
	"os"
	"path/filepath"
)

// Explicit failed-bootstrap evidence only. It cannot register, put, delete,
// change protection, or resend any prior physical attempt.
func inspectFailed(ctx context.Context, c config) error {
	path := filepath.Join(c.OutputDirectory, "failed-native-inventory.json")
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		return errors.New("failed inventory already recorded")
	}
	t := &bootstrapTransport{host: c.Endpoint, receipts: filepath.Join(c.OutputDirectory, "bootstrap-receipts.jsonl"), base: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	client, e := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: t})
	if e != nil {
		return e
	}
	var objects []any
	for _, ns := range namespaces {
		if e = one(ctx, "failed-readonly-version-inventory:"+ns, func(x context.Context) error {
			n := 0
			for o := range client.ListObjects(x, ns, minio.ListObjectsOptions{WithVersions: true, Recursive: true, MaxKeys: 32}) {
				if o.Err != nil {
					return o.Err
				}
				if (o.Key != "fixture/a.csv" && o.Key != "fixture/b.csv") || o.VersionID == "" || o.VersionID == "null" || o.IsDeleteMarker {
					return errors.New("failed inventory outside exact synthetic scope")
				}
				n++
				objects = append(objects, map[string]any{"namespace": ns, "key": o.Key, "version": o.VersionID, "bytes": o.Size})
			}
			if n != 2 {
				return errors.New("expected two retained versions per namespace")
			}
			return nil
		}); e != nil {
			return e
		}
	}
	b, e := json.MarshalIndent(map[string]any{"objects": objects, "registry": "transaction rolled back", "delete": 0, "cleanup": 0}, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
