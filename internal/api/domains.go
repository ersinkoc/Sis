package api

import (
	"github.com/ersinkoc/sis/internal/policy"
)

func normalizeDomainInput(domain string) (string, bool) {
	normalized, ok := policy.NormalizeDomainPattern(domain)
	if !ok {
		return "", false
	}
	return normalized, true
}
