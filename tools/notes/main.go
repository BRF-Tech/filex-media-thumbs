// Command notes prints the CHANGELOG.md section of a version: the body of
// its GitHub release, which is what filex shows an administrator under
// "Release notes" when it offers the update (Admin → Plugins → Apps →
// Review update).
//
//	go run ./tools/notes 0.1.0 > dist/release-notes.md
//
// A version with no section, or an empty one, is an error, not an empty
// body: a release without its changelog entry is a mistake to fix before the
// tag.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/notes <version>")
		os.Exit(2)
	}
	version := strings.TrimPrefix(os.Args[1], "v")
	src, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "notes:", err)
		os.Exit(1)
	}
	body, ok := section(string(src), version)
	if !ok {
		fmt.Fprintf(os.Stderr, "notes: CHANGELOG.md has no \"## [%s]\" section with text in it: write what %s changes before tagging it\n", version, version)
		os.Exit(1)
	}
	fmt.Println(body)
}

// section is the body of `## [<version>]` (or `## <version>`) in changelog,
// trimmed, up to the next `## ` heading.
func section(changelog, version string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n")
	head := regexp.MustCompile(`^## \[?v?` + regexp.QuoteMeta(version) + `\]?(\s|$)`)
	start := -1
	for i, l := range lines {
		if head.MatchString(l) {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	body := strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
	return body, body != ""
}
