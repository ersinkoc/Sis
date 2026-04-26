package policy

import "fmt"

func errUnknownList(id string) error {
	return fmt.Errorf("blocklist %q not found", id)
}

func errDisabledList(id string) error {
	return fmt.Errorf("blocklist %q is disabled", id)
}
