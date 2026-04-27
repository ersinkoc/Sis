package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDocumentIncludesGoAndNpmPackages(t *testing.T) {
	t.Chdir(repoRoot(t))

	doc, err := buildDocument("v1.2.3", "abc1234", "2026-04-27T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if doc.SPDXVersion != "SPDX-2.3" {
		t.Fatalf("SPDXVersion = %q", doc.SPDXVersion)
	}
	if doc.Packages[0].Name != "sis" || doc.Packages[0].VersionInfo != "v1.2.3" {
		t.Fatalf("root package = %#v", doc.Packages[0])
	}
	if !hasPURL(doc.Packages, "pkg:golang/github.com/miekg/dns@v1.1.72") {
		t.Fatal("missing Go module purl")
	}
	if !hasPURLPrefix(doc.Packages, "pkg:npm/react@") {
		t.Fatal("missing npm package purl")
	}
	if len(doc.Relationships) != len(doc.Packages)-1 {
		t.Fatalf("relationships=%d packages=%d", len(doc.Relationships), len(doc.Packages))
	}
}

func TestNPMLockPackagesHandlesScopedNamesAndLicenses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package-lock.json")
	raw := `{
  "packages": {
    "": {"name": "@sis/webui", "version": "0.1.0"},
    "node_modules/@scope/pkg": {"version": "1.2.3", "license": "MIT", "resolved": "https://registry.example/pkg.tgz"},
    "node_modules/plain": {"name": "plain", "version": "2.0.0", "license": ["MIT", "Apache-2.0"]}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	packages, err := npmLockPackages(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 {
		t.Fatalf("packages = %#v", packages)
	}
	if !hasPURL(packages, "pkg:npm/@scope/pkg@1.2.3") {
		t.Fatalf("missing scoped package: %#v", packages)
	}
	for _, p := range packages {
		if p.Name == "plain" && p.LicenseDeclared != "MIT AND Apache-2.0" {
			t.Fatalf("license = %q", p.LicenseDeclared)
		}
	}
}

func TestHelpers(t *testing.T) {
	if got := sanitizeID("@scope/pkg v1.0.0"); got != "scope-pkg-v1.0.0" {
		t.Fatalf("sanitizeID = %q", got)
	}
	if got := sanitizeID("..."); got != "unknown" {
		t.Fatalf("empty sanitizeID = %q", got)
	}
	if got := downloadLocation(""); got != "NOASSERTION" {
		t.Fatalf("downloadLocation = %q", got)
	}
	if got := licenseString(123); got != "" {
		t.Fatalf("licenseString = %q", got)
	}
	if got := checksumLike("abc1234"); len(got) != 40 {
		t.Fatalf("checksumLike length = %d", len(got))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}

func hasPURL(packages []pkg, target string) bool {
	for _, p := range packages {
		for _, ref := range p.ExternalRefs {
			if ref.ReferenceLocator == target {
				return true
			}
		}
	}
	return false
}

func hasPURLPrefix(packages []pkg, prefix string) bool {
	for _, p := range packages {
		for _, ref := range p.ExternalRefs {
			if strings.HasPrefix(ref.ReferenceLocator, prefix) {
				return true
			}
		}
	}
	return false
}
