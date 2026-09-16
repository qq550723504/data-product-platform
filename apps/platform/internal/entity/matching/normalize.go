package matching

import (
	"strings"
	"unicode"
)

type CompanyRecord struct {
	SourceCompanyID         string
	CompanyName             string
	UnifiedSocialCreditCode string
	LegalRepresentative     string
	RegisteredAddress       string
	EntryDate               string
	CompanyStatus           string
	Raw                     map[string]string
}

type NormalizedCompany struct {
	SourceCompanyID         string
	CompanyName             string
	UnifiedSocialCreditCode string
	LegalRepresentative     string
	RegisteredAddress       string
	EntryDate               string
	CompanyStatus           string
}

func NormalizeCompany(record CompanyRecord, policy Policy) NormalizedCompany {
	return NormalizedCompany{
		SourceCompanyID:         strings.TrimSpace(record.SourceCompanyID),
		CompanyName:             normalizeCompanyName(record.CompanyName, policy),
		UnifiedSocialCreditCode: normalizeUSCC(record.UnifiedSocialCreditCode, policy),
		LegalRepresentative:     strings.TrimSpace(record.LegalRepresentative),
		RegisteredAddress:       normalizeAddress(record.RegisteredAddress, policy),
		EntryDate:               strings.TrimSpace(record.EntryDate),
		CompanyStatus:           strings.TrimSpace(record.CompanyStatus),
	}
}

func normalizeCompanyName(value string, policy Policy) string {
	if policy.Spec.Normalization.CompanyName.NormalizeFullWidthHalfWidth {
		value = normalizeFullWidth(value)
	}
	if policy.Spec.Normalization.CompanyName.Trim {
		value = strings.TrimSpace(value)
	}
	if policy.Spec.Normalization.CompanyName.NormalizeWhitespace {
		value = removeWhitespace(value)
	}
	for _, alias := range policy.Spec.Normalization.CompanyName.Aliases {
		if alias.Suffix != "" && strings.HasSuffix(value, alias.Suffix) {
			value = strings.TrimSuffix(value, alias.Suffix) + alias.Canonical
		}
	}
	return value
}

func normalizeAddress(value string, policy Policy) string {
	if policy.Spec.Normalization.Address.Trim {
		value = strings.TrimSpace(value)
	}
	if policy.Spec.Normalization.Address.NormalizeWhitespace {
		value = removeWhitespace(value)
	}
	return value
}

func normalizeUSCC(value string, policy Policy) string {
	if policy.Spec.Normalization.UnifiedSocialCreditCode.Trim {
		value = strings.TrimSpace(value)
	}
	if policy.Spec.Normalization.UnifiedSocialCreditCode.Uppercase {
		value = strings.ToUpper(value)
	}
	return value
}

func removeWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func normalizeFullWidth(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\u3000':
			return ' '
		case r >= '\uFF01' && r <= '\uFF5E':
			return r - 0xFEE0
		default:
			return r
		}
	}, value)
}

func Similarity(a, b string) float64 {
	ar := []rune(a)
	br := []rune(b)
	maxLen := len(ar)
	if len(br) > maxLen {
		maxLen = len(br)
	}
	if maxLen == 0 {
		return 1
	}
	distance := levenshtein(ar, br)
	return 1 - float64(distance)/float64(maxLen)
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min3(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
