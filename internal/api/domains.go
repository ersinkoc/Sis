package api

import (
	"strings"

	"github.com/ersinkoc/sis/internal/policy"
)

func normalizeDomainInput(domain string) (string, bool) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")
	if strings.TrimSpace(domain) != domain {
		return "", false
	}
	if strings.HasSuffix(domain, ".") {
		return "", false
	}
	if domain == "" {
		return "", false
	}
	domains := policy.NewDomains()
	if !domains.Add(domain) {
		return "", false
	}
	return domain, true
}
