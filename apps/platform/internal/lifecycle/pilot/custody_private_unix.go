//go:build !windows

package pilot

import (
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"os"
)

func custodyPrivateACL(_ string, info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return lc.ErrCustodyScope
	}
	return nil
}
