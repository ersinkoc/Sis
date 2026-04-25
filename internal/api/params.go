package api

import (
	"fmt"
	"net/http"
	"strconv"
)

func intQuery(r *http.Request, name string, fallback, max int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	if max > 0 && value > max {
		value = max
	}
	return value, nil
}
