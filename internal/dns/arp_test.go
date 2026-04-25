package dns

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinuxARP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arp")
	content := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.10     0x1         0x2         AA:BB:CC:DD:EE:FF     *        eth0
192.168.1.11     0x1         0x0         00:00:00:00:00:00     *        eth0
`
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := parseLinuxARP(f)
	if err != nil {
		t.Fatal(err)
	}
	if got["192.168.1.10"] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("entries = %#v", got)
	}
	if _, ok := got["192.168.1.11"]; ok {
		t.Fatalf("zero mac should be skipped: %#v", got)
	}
}
