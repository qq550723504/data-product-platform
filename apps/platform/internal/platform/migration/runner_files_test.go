package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerRejectsDuplicateMigrationVersionsBothDirections(t *testing.T) {
	for _, suffix := range []string{".up.sql", ".down.sql"} {
		t.Run(suffix, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"000054_one", "000054_two"} {
				if err := os.WriteFile(filepath.Join(dir, name+suffix), []byte("SELECT 1;"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := NewRunner(nil, dir).files(suffix)
			if err == nil || !strings.Contains(err.Error(), "duplicate migration version 54") {
				t.Fatalf("duplicate silently accepted: %v", err)
			}
		})
	}
}

func TestRunnerAllowsSameVersionUpDownPairAndOrdersGaps(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"000049_previous.up.sql", "000054_current.up.sql", "000054_current.down.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := NewRunner(nil, dir).files(".up.sql")
	if err != nil || len(files) != 2 || files[0].Version != 49 || files[1].Version != 54 {
		t.Fatalf("normal pair/gap rejected: %+v %v", files, err)
	}
}
