// Independent read-only acceptance oracle; never a trial startup dependency.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

func must(e error) {
	if e != nil {
		panic("read-only oracle failed")
	}
}
func load(p string, v any) { b, e := os.ReadFile(p); must(e); must(json.Unmarshal(b, v)) }
func parse(b []byte) [][]string {
	v, e := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte{239, 187, 191}))).ReadAll()
	must(e)
	return v
}
func sorted(v [][]string) [][]string {
	sort.Slice(v, func(i, j int) bool {
		a, _ := json.Marshal(v[i])
		b, _ := json.Marshal(v[j])
		return string(a) < string(b)
	})
	return v
}
func main() {
	var s struct{ Private string }
	load(os.Args[1], &s)
	var c struct {
		Repo, RootKey, RootSecret string
		Profile                   struct {
			StorageEndpoint string
			Tables          []struct {
				Code, Name string
				Columns    []string
				Rows       int
			}
		}
	}
	load(filepath.Join(s.Private, "config.json"), &c)
	var f struct {
		Facts struct {
			Operations []struct {
				ID           string
				Manifest     d.Manifest
				ManifestHash string `json:"manifest_hash"`
			}
			Versions []struct {
				ID, Status string
				VersionNo  int `json:"version_no"`
			}
		}
		Objects []struct {
			Code, URI, SHA256 string
			Rows              int
		}
	}
	load(os.Args[2], &f)
	if len(f.Facts.Operations) != 1 || len(f.Facts.Versions) != 3 || len(f.Objects) != 3 {
		panic("exact one operation / three versions / three objects required")
	}
	op := f.Facts.Operations[0]
	_, h, e := op.Manifest.Canonical()
	must(e)
	if h != op.ManifestHash {
		panic("manifest hash mismatch")
	}
	for _, v := range f.Facts.Versions {
		if v.Status != "READY" || v.VersionNo != 1 {
			panic("wrong RAW lifecycle")
		}
	}
	client, e := minio.New(c.Profile.StorageEndpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootKey, c.RootSecret, ""), Secure: false})
	must(e)
	report := map[string]any{"manifestHash": h, "operationId": op.ID}
	outputs := []any{}
	for _, t := range c.Profile.Tables {
		var uri, expected string
		for _, o := range f.Objects {
			if o.Code == t.Code {
				uri = o.URI
				expected = o.SHA256
			}
		}
		if !strings.HasPrefix(uri, "s3://raw/core/") {
			panic("wrong native Core prefix")
		}
		obj, e := client.GetObject(context.Background(), "raw", strings.TrimPrefix(uri, "s3://raw/"), minio.GetObjectOptions{})
		must(e)
		b, e := io.ReadAll(obj)
		must(e)
		must(obj.Close())
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != expected {
			panic("native payload checksum mismatch")
		}
		actual := parse(b)
		fixture, e := os.ReadFile(filepath.Join(c.Repo, "examples/enterprise-activity/data", t.Name+".csv"))
		must(e)
		want := parse(fixture)
		if !reflect.DeepEqual(actual[0], t.Columns) || !reflect.DeepEqual(want[0], t.Columns) || len(actual) != t.Rows+1 || !reflect.DeepEqual(sorted(actual[1:]), sorted(want[1:])) {
			panic("actual values/schema differ from frozen fixture")
		}
		found := false
		for _, o := range op.Manifest.Outputs {
			if o.Code == t.Code && o.SHA256 == expected && o.Rows == int64(t.Rows) && reflect.DeepEqual(o.Columns, t.Columns) {
				found = true
			}
		}
		if !found {
			panic("manifest/schema/object mismatch")
		}
		outputs = append(outputs, map[string]any{"code": t.Code, "rows": t.Rows, "sha256": expected, "bytes": len(b), "schemaAndValues": "PASS"})
	}
	report["outputs"] = outputs
	report["nativeObjectGET"] = "PASS"
	report["fixtureValues"] = "PASS"
	must(json.NewEncoder(os.Stdout).Encode(report))
	fmt.Fprintln(os.Stderr, "read-only oracle PASS")
}
