package policy

import "fmt"

func errUnknownList(id string) error {
	return fmt.Errorf("blocklist %q not found", id)
}
