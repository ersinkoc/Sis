package api

import (
	"strings"

	"github.com/ersinkoc/sis/internal/policy"
)

func normalizeDomainInput(domain string) (string, bool) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return "", false
	}
	domains := policy.NewDomains()
	if !domains.Add(domain) {
		return "", false
	}
	return domain, true
}
