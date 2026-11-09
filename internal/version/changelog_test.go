package version

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The release workflow publishes a version only when CHANGELOG.md has a line
// that is exactly "## <version>". A heading that is slightly off (stray text,
// a section pasted into a paragraph) silently means "no release", so every
// "## " line must be a well-formed section heading.
var headingPattern = regexp.MustCompile(`^## (Unreleased|v(\d{4})\.(\d{1,2})\.(\d+))$`)

// changelogProblems lists malformed, duplicate or out-of-order headings.
func changelogProblems(text string) []string {
	var problems []string
	seen := map[string]bool{}
	var prev [3]int
	sc := bufio.NewScanner(strings.NewReader(text))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		m := headingPattern.FindStringSubmatch(line)
		if m == nil {
			problems = append(problems, fmt.Sprintf("line %d: %q is not \"## Unreleased\" or \"## vYEAR.MONTH.PATCH\", so it can't cut a release", n, line))
			continue
		}
		if seen[m[1]] {
			problems = append(problems, fmt.Sprintf("line %d: duplicate section %s", n, m[1]))
		}
		seen[m[1]] = true
		if m[1] == "Unreleased" {
			continue
		}
		var v [3]int
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+2])
		}
		if prev != ([3]int{}) && !older(v, prev) {
			problems = append(problems, fmt.Sprintf("line %d: %s is out of order; newest first", n, m[1]))
		}
		prev = v
	}
	return problems
}

func older(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func TestChangelogHeadings(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range changelogProblems(string(b)) {
		t.Errorf("CHANGELOG.md %s", p)
	}
}

func TestChangelogProblems(t *testing.T) {
	cases := []struct {
		name, text string
		bad        bool
	}{
		{"valid", "# Changelog\n\n## Unreleased\n\n## v2026.11.6\n\n## v2026.11.4\n", false},
		{"section pasted into a paragraph", "such as `## v2026.11.5\n\n## v2026.11.40`, is what\n## v2026.11.4\n", true},
		{"trailing text", "## v2026.11.6 (beta)\n", true},
		{"out of order", "## v2026.11.4\n## v2026.11.6\n", true},
		{"duplicate", "## v2026.11.4\n## v2026.11.4\n", true},
		{"month before year order", "## v2026.12.1\n## v2026.11.9\n", false},
	}
	for _, c := range cases {
		got := changelogProblems(c.text)
		if (len(got) > 0) != c.bad {
			t.Errorf("%s: problems = %q, want bad=%v", c.name, got, c.bad)
		}
	}
}
