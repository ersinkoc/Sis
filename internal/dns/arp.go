package dns

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type ARPTable struct {
	entries atomic.Value
	refresh time.Duration
}

func NewARPTable(refresh time.Duration) *ARPTable {
	if refresh <= 0 {
		refresh = 30 * time.Second
	}
	t := &ARPTable{refresh: refresh}
	t.entries.Store(map[string]string{})
	return t
}

func (t *ARPTable) Lookup(ip net.IP) (string, bool) {
	if t == nil || ip == nil {
		return "", false
	}
	entries, _ := t.entries.Load().(map[string]string)
	mac, ok := entries[ip.String()]
	return mac, ok
}

func (t *ARPTable) Refresh() error {
	entries := make(map[string]string)
	ipv4, err := parseLinuxARPFile("/proc/net/arp")
	if err == nil {
		for ip, mac := range ipv4 {
			entries[ip] = mac
		}
	}
	ipv6, err6 := readLinuxNDP("/sys/class/net")
	if err6 == nil {
		for ip, mac := range ipv6 {
			entries[ip] = mac
		}
	}
	t.entries.Store(entries)
	if err != nil && err6 != nil {
		return err
	}
	return nil
}

func (t *ARPTable) Run(ctx context.Context) {
	_ = t.Refresh()
	ticker := time.NewTicker(t.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = t.Refresh()
		}
	}
}

func parseLinuxARPFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseLinuxARP(f)
}

func parseLinuxARP(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(r)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := net.ParseIP(fields[0])
		mac, err := net.ParseMAC(fields[3])
		if ip == nil || err != nil || isZeroMAC(mac) {
			continue
		}
		out[ip.String()] = strings.ToLower(mac.String())
	}
	return out, scanner.Err()
}

func readLinuxNDP(sysClassNet string) (map[string]string, error) {
	out := make(map[string]string)
	ifaces, err := os.ReadDir(sysClassNet)
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		neighDir := filepath.Join(sysClassNet, iface.Name(), "neigh")
		entries, err := os.ReadDir(neighDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			ip := net.ParseIP(entry.Name())
			if ip == nil {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(neighDir, entry.Name(), "lladdr"))
			if err != nil {
				continue
			}
			mac, err := net.ParseMAC(strings.TrimSpace(string(raw)))
			if err != nil || isZeroMAC(mac) {
				continue
			}
			out[ip.String()] = strings.ToLower(mac.String())
		}
	}
	return out, nil
}

func isZeroMAC(mac net.HardwareAddr) bool {
	if len(mac) == 0 {
		return true
	}
	for _, b := range mac {
		if b != 0 {
			return false
		}
	}
	return true
}
