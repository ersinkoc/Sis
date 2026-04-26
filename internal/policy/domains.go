package policy

import (
	"sort"
	"strings"
)

// Domains is a suffix-matching domain trie.
type Domains struct {
	root node
	size int
}

type node struct {
	children map[string]*node
	terminal bool
}

// NewDomains creates an empty domain set.
func NewDomains() *Domains {
	return &Domains{}
}

// Add inserts a domain or wildcard suffix into the set.
func (d *Domains) Add(domain string) bool {
	labels, ok := labelsFor(domain)
	if !ok {
		return false
	}
	cur := &d.root
	for _, label := range labels {
		if cur.children == nil {
			cur.children = make(map[string]*node)
		}
		next := cur.children[label]
		if next == nil {
			next = &node{}
			cur.children[label] = next
		}
		cur = next
	}
	if cur.terminal {
		return false
	}
	cur.terminal = true
	d.size++
	return true
}

// Match reports whether domain matches an inserted domain suffix.
func (d *Domains) Match(domain string) bool {
	if d == nil {
		return false
	}
	labels, ok := labelsFor(domain)
	if !ok {
		return false
	}
	cur := &d.root
	for _, label := range labels {
		if cur.terminal {
			return true
		}
		if cur.children == nil {
			return false
		}
		next := cur.children[label]
		if next == nil {
			return false
		}
		cur = next
	}
	return cur.terminal
}

// Delete removes an exact domain suffix from the set.
func (d *Domains) Delete(domain string) bool {
	if d == nil {
		return false
	}
	labels, ok := labelsFor(domain)
	if !ok {
		return false
	}
	cur := &d.root
	for _, label := range labels {
		if cur.children == nil {
			return false
		}
		next := cur.children[label]
		if next == nil {
			return false
		}
		cur = next
	}
	if !cur.terminal {
		return false
	}
	cur.terminal = false
	d.size--
	return true
}

// Len returns the number of inserted domain suffixes.
func (d *Domains) Len() int {
	if d == nil {
		return 0
	}
	return d.size
}

// Entries returns up to limit domain entries containing query.
func (d *Domains) Entries(query string, limit int) []string {
	if d == nil {
		return []string{}
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	query = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(query, ".")))
	out := make([]string, 0, min(limit, d.size))
	d.root.entries(nil, query, limit, &out)
	return out
}

func (n *node) entries(path []string, query string, limit int, out *[]string) {
	if n == nil || len(*out) >= limit {
		return
	}
	if n.terminal {
		domain := domainFromLabels(path)
		if query == "" || strings.Contains(domain, query) {
			*out = append(*out, domain)
			if len(*out) >= limit {
				return
			}
		}
	}
	if len(n.children) == 0 {
		return
	}
	keys := make([]string, 0, len(n.children))
	for key := range n.children {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		n.children[key].entries(append(path, key), query, limit, out)
		if len(*out) >= limit {
			return
		}
	}
}

func domainFromLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, len(labels))
	for i := range labels {
		parts[i] = labels[len(labels)-1-i]
	}
	return strings.Join(parts, ".")
}

func labelsFor(domain string) ([]string, bool) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	domain = strings.TrimPrefix(domain, "*.")
	if domain == "" || strings.Contains(domain, " ") {
		return nil, false
	}
	parts := strings.Split(domain, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	for _, part := range parts {
		if !validLabel(part) {
			return nil, false
		}
	}
	return parts, true
}

func validLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
