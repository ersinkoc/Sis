package policy

import (
	"bufio"
	"io"
	"net"
	"strings"
)

// ParseStats summarizes blocklist parsing results.
type ParseStats struct {
	Lines     int
	Accepted  int
	Skipped   int
	Malformed int
}

// ParseBlocklist parses hosts-style or domain-only blocklist content.
func ParseBlocklist(r io.Reader) (*Domains, ParseStats, error) {
	domains := NewDomains()
	var stats ParseStats
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		stats.Lines++
		line := stripComment(scanner.Text())
		if line == "" {
			stats.Skipped++
			continue
		}
		domain, ok := domainFromLine(line)
		if !ok {
			stats.Malformed++
			continue
		}
		if domains.Add(domain) {
			stats.Accepted++
		} else {
			stats.Skipped++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, stats, err
	}
	return domains, stats, nil
}

func stripComment(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func domainFromLine(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false
	}
	if len(fields) == 1 {
		if net.ParseIP(fields[0]) != nil {
			return "", false
		}
		if _, ok := labelsFor(fields[0]); !ok {
			return "", false
		}
		return fields[0], true
	}
	if net.ParseIP(fields[0]) == nil {
		return "", false
	}
	for _, field := range fields[1:] {
		if net.ParseIP(field) != nil {
			continue
		}
		if _, ok := labelsFor(field); ok {
			return field, true
		}
	}
	return "", false
}
