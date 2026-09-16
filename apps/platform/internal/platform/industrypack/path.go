package industrypack

import (
	"fmt"
	"path/filepath"
	"strings"
)

func ResolvePath(root, ref string) (string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("resolve industry pack root: %w", err)
	}
	cleanRef := filepath.Clean(strings.TrimSpace(ref))
	if cleanRef == "." || filepath.IsAbs(cleanRef) {
		return "", fmt.Errorf("industry pack ref must be a relative path")
	}
	candidate, err := filepath.Abs(filepath.Join(rootAbs, cleanRef))
	if err != nil {
		return "", fmt.Errorf("resolve industry pack ref: %w", err)
	}
	relative, err := filepath.Rel(rootAbs, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("industry pack ref escapes configured root")
	}
	return candidate, nil
}
