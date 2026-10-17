// Offline diagnosis only: official MinIO policy semantics, no provider client.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/minio/pkg/v3/policy"
)

func equivalent(expected, current []byte) bool {
	a, err := policy.ParseConfig(bytes.NewReader(expected))
	if err != nil {
		return false
	}
	b, err := policy.ParseConfig(bytes.NewReader(current))
	return err == nil && a.Equals(*b)
}

func main() {
	files, err := filepath.Glob(filepath.Join(os.Args[2], "native-policy-*.json"))
	if err != nil || len(files) != 4 {
		panic("exact four business policies required")
	}
	results := []map[string]any{}
	all := true
	for _, path := range files {
		old, e := os.ReadFile(filepath.Join(os.Args[1], filepath.Base(path)))
		if e != nil {
			panic(e)
		}
		fresh, e := os.ReadFile(path)
		if e != nil {
			panic(e)
		}
		equal := equivalent(old, fresh)
		before, after := sha256.Sum256(old), sha256.Sum256(fresh)
		results = append(results, map[string]any{"policy": filepath.Base(path), "old_sha256": hex.EncodeToString(before[:]), "fresh_sha256": hex.EncodeToString(after[:]), "raw_equal": bytes.Equal(old, fresh), "official_policy_equals": equal})
		all = all && equal
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"all_official_equals": all, "policies": results, "provider_io": 0, "native_delete": 0})
	if !all {
		os.Exit(1)
	}
}
