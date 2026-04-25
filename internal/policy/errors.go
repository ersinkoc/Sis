package policy

import "fmt"

func errInvalidDomain(path, domain string) error {
	return fmt.Errorf("%s: invalid domain %q", path, domain)
}
