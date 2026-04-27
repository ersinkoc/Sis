package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type document struct {
	SPDXVersion       string         `json:"spdxVersion"`
	DataLicense       string         `json:"dataLicense"`
	SPDXID            string         `json:"SPDXID"`
	Name              string         `json:"name"`
	DocumentNamespace string         `json:"documentNamespace"`
	CreationInfo      creationInfo   `json:"creationInfo"`
	Packages          []pkg          `json:"packages"`
	Relationships     []relationship `json:"relationships"`
}

type creationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type pkg struct {
	Name                    string              `json:"name"`
	SPDXID                  string              `json:"SPDXID"`
	VersionInfo             string              `json:"versionInfo,omitempty"`
	DownloadLocation        string              `json:"downloadLocation"`
	FilesAnalyzed           bool                `json:"filesAnalyzed"`
	LicenseConcluded        string              `json:"licenseConcluded"`
	LicenseDeclared         string              `json:"licenseDeclared"`
	CopyrightText           string              `json:"copyrightText"`
	ExternalRefs            []externalReference `json:"externalRefs,omitempty"`
	PackageVerificationCode *verificationCode   `json:"packageVerificationCode,omitempty"`
}

type verificationCode struct {
	PackageVerificationCodeValue string `json:"packageVerificationCodeValue"`
}

type externalReference struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type relationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

type moduleInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Main    bool   `json:"Main"`
	Replace *struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
	} `json:"Replace"`
}

type packageLock struct {
	Packages map[string]struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		License  any    `json:"license"`
		Resolved string `json:"resolved"`
		Dev      bool   `json:"dev"`
	} `json:"packages"`
}

func main() {
	out := flag.String("out", "dist/sis.spdx.json", "output SPDX JSON path")
	version := flag.String("version", "dev", "application version")
	commit := flag.String("commit", "none", "source commit")
	date := flag.String("date", time.Now().UTC().Format(time.RFC3339), "creation timestamp")
	flag.Parse()

	doc, err := buildDocument(*version, *commit, *date)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildDocument(version, commit, date string) (*document, error) {
	if _, err := time.Parse(time.RFC3339, date); err != nil {
		return nil, fmt.Errorf("invalid SPDX creation date %q: %w", date, err)
	}
	doc := &document{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "Sis " + version,
		DocumentNamespace: "https://github.com/ersinkoc/Sis/sbom/" + version + "/" + commit,
		CreationInfo: creationInfo{
			Created:  date,
			Creators: []string{"Tool: sis-sbom"},
		},
	}
	root := pkg{
		Name:             "sis",
		SPDXID:           "SPDXRef-Package-sis",
		VersionInfo:      version,
		DownloadLocation: "https://github.com/ersinkoc/Sis",
		FilesAnalyzed:    false,
		LicenseConcluded: "NOASSERTION",
		LicenseDeclared:  "NOASSERTION",
		CopyrightText:    "NOASSERTION",
		ExternalRefs: []externalReference{{
			ReferenceCategory: "PACKAGE-MANAGER",
			ReferenceType:     "purl",
			ReferenceLocator:  "pkg:github/ersinkoc/Sis@" + version,
		}},
		PackageVerificationCode: &verificationCode{PackageVerificationCodeValue: checksumLike(commit)},
	}
	doc.Packages = append(doc.Packages, root)

	goPackages, err := goModulePackages()
	if err != nil {
		return nil, err
	}
	npmPackages, err := npmLockPackages("webui/package-lock.json")
	if err != nil {
		return nil, err
	}
	doc.Packages = append(doc.Packages, goPackages...)
	doc.Packages = append(doc.Packages, npmPackages...)
	sort.Slice(doc.Packages[1:], func(i, j int) bool {
		return doc.Packages[i+1].SPDXID < doc.Packages[j+1].SPDXID
	})
	for _, p := range doc.Packages[1:] {
		doc.Relationships = append(doc.Relationships, relationship{
			SPDXElementID:      root.SPDXID,
			RelationshipType:   "DEPENDS_ON",
			RelatedSPDXElement: p.SPDXID,
		})
	}
	return doc, nil
}

func goModulePackages() ([]pkg, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	raw, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("go list modules failed: %s", bytes.TrimSpace(exit.Stderr))
		}
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var out []pkg
	for {
		var mod moduleInfo
		if err := dec.Decode(&mod); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if mod.Main {
			continue
		}
		path, version := mod.Path, mod.Version
		if mod.Replace != nil {
			path = mod.Replace.Path
			if mod.Replace.Version != "" {
				version = mod.Replace.Version
			}
		}
		out = append(out, packageEntry("go", path, version, "https://"+path, "pkg:golang/"+path+"@"+version))
	}
	return out, nil
}

func npmLockPackages(path string) ([]pkg, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock packageLock
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []pkg
	for lockPath, entry := range lock.Packages {
		if lockPath == "" || entry.Version == "" {
			continue
		}
		name := entry.Name
		if name == "" {
			name = strings.TrimPrefix(lockPath, "node_modules/")
		}
		key := name + "@" + entry.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		p := packageEntry("npm", name, entry.Version, downloadLocation(entry.Resolved), "pkg:npm/"+name+"@"+entry.Version)
		if license := licenseString(entry.License); license != "" {
			p.LicenseDeclared = license
		}
		out = append(out, p)
	}
	return out, nil
}

func packageEntry(kind, name, version, location, purl string) pkg {
	return pkg{
		Name:             name,
		SPDXID:           "SPDXRef-Package-" + kind + "-" + sanitizeID(name+"-"+version),
		VersionInfo:      version,
		DownloadLocation: location,
		FilesAnalyzed:    false,
		LicenseConcluded: "NOASSERTION",
		LicenseDeclared:  "NOASSERTION",
		CopyrightText:    "NOASSERTION",
		ExternalRefs: []externalReference{{
			ReferenceCategory: "PACKAGE-MANAGER",
			ReferenceType:     "purl",
			ReferenceLocator:  purl,
		}},
	}
}

func downloadLocation(value string) string {
	if value == "" {
		return "NOASSERTION"
	}
	return value
}

func licenseString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " AND ")
	default:
		return ""
	}
}

var idPattern = regexp.MustCompile(`[^A-Za-z0-9.-]+`)

func sanitizeID(value string) string {
	value = idPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	if value == "" {
		return "unknown"
	}
	return value
}

func checksumLike(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
