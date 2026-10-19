package config

import "testing"

func TestSyntheticRuntimeCannotFallBackToRootStorage(t *testing.T) {
	t.Setenv("APP_ENV", "synthetic-trial")
	t.Setenv("POSTGRES_DSN", "controlled")
	t.Setenv("OBJECT_STORAGE_WORKSPACE_PROFILE", "")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "configured-root")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "configured-secret")
	if _, err := Load(); err == nil {
		t.Fatal("synthetic runtime accepted root fallback")
	}
	t.Setenv("OBJECT_STORAGE_WORKSPACE_PROFILE", "private-profile.json")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
