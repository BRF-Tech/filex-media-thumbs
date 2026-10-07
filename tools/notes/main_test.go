package main

import (
	"os"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

## [0.2.0] - 2026-11-01

### Added

- Krita files.

## [0.1.0] - 2026-10-06

### Added

- The first version.
`

func TestSection(t *testing.T) {
	got, ok := section(changelog, "0.2.0")
	if !ok || got != "### Added\n\n- Krita files." {
		t.Fatalf("0.2.0: %q", got)
	}
	got, ok = section(changelog, "0.1.0")
	if !ok || got != "### Added\n\n- The first version." {
		t.Fatalf("0.1.0: %q", got)
	}
	if _, ok := section(changelog, "0.3.0"); ok {
		t.Fatal("a version without a section")
	}
	if _, ok := section(changelog, "Unreleased"); ok {
		t.Fatal("an empty section is no release notes")
	}
	if _, ok := section(changelog, "0.1"); ok {
		t.Fatal("0.1 is not 0.1.0")
	}
}

// The repository's own CHANGELOG.md says what the manifest's version is.
func TestTheChangelogHasTheVersion(t *testing.T) {
	src, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := section(string(src), "0.1.0"); !ok {
		t.Fatal("CHANGELOG.md has no 0.1.0 section")
	}
}
