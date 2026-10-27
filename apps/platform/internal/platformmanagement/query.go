// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package platformmanagement

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
)

// Fold is stateless and concurrency-safe; do not replace it with lowercasing,
// which misses equivalent Unicode case forms such as Greek sigma.
var directorySearchFold = cases.Fold()

func (p PageRequest) Normalize() (PageRequest, error) {
	if p.Limit < 1 || p.Limit > 100 || p.Offset < 0 || p.Offset > 10000 ||
		len(p.Filter.Search) > 200 || !utf8.ValidString(p.Filter.Search) || strings.ContainsFunc(p.Filter.Search, unicode.IsControl) {
		return PageRequest{}, ErrInvalidRequest
	}
	if p.Filter.Role != "" && !IsIdentifier(p.Filter.Role) {
		return PageRequest{}, ErrInvalidRequest
	}
	if p.Filter.State != "" && p.Filter.State != "active" && p.Filter.State != "inactive" {
		return PageRequest{}, ErrInvalidRequest
	}
	p.Filter.Search = strings.TrimSpace(p.Filter.Search)
	return p, nil
}

// Matches consumes the current directory projection, never a contact profile or
// an inferred role. Search is literal Unicode substring matching, without SQL
// wildcards, regular expressions or accent folding.
func (f ListFilter) Matches(member Member) bool {
	if (f.Role != "" && !slices.Contains(member.Roles, f.Role)) || (f.State != "" && member.State != f.State) {
		return false
	}
	if f.Search == "" {
		return true
	}
	q := directorySearchFold.String(f.Search)
	return strings.Contains(directorySearchFold.String(member.DisplayName), q) || strings.Contains(directorySearchFold.String(member.LoginName), q)
}
