package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fixtureDirs(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location unavailable")
	}
	dir := filepath.Dir(file)
	return filepath.Join(dir, "..", "remaining-candidate07", "business-races", "readonly"), filepath.Join(dir, "business-races", "readonly")
}

func TestRecordedPoliciesUseOfficialSetEquality(t *testing.T) {
	oldDir, freshDir := fixtureDirs(t)
	files, err := filepath.Glob(filepath.Join(freshDir, "native-policy-*.json"))
	if err != nil || len(files) != 4 {
		t.Fatal("exact four recorded policies required")
	}
	changed := 0
	for _, path := range files {
		old, err := os.ReadFile(filepath.Join(oldDir, filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(old, current) {
			changed++
		}
		if !equivalent(old, current) {
			t.Fatalf("recorded policy not semantically equal: %s", filepath.Base(path))
		}
	}
	if changed != 3 {
		t.Fatalf("expected three serialization changes, got %d", changed)
	}
}

func TestWiderOrDifferentPoliciesRemainRejected(t *testing.T) {
	oldDir, _ := fixtureDirs(t)
	old, err := os.ReadFile(filepath.Join(oldDir, "native-policy-lifecycle307-remaining-business-remainread808a6960.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"action", "resource", "version"} {
		t.Run(kind, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(old, &doc); err != nil {
				t.Fatal(err)
			}
			statement := doc["Statement"].([]any)[0].(map[string]any)
			switch kind {
			case "action":
				statement["Action"] = []string{"s3:GetObjectVersion", "s3:DeleteObjectVersion"}
			case "resource":
				statement["Resource"] = []string{"arn:aws:s3:::foreign/*"}
			case "version":
				statement["Condition"] = map[string]any{"StringEquals": map[string]any{"s3:versionid": "unfrozen-version"}}
			}
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if equivalent(old, raw) {
				t.Fatal("changed authorization policy accepted")
			}
		})
	}
	if equivalent(old, []byte("invalid")) || equivalent([]byte("invalid"), old) {
		t.Fatal("malformed policy accepted")
	}
}
