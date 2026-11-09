package version

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scripts/version.sh computes the ingredients and this package renders them.
// The two must agree: the release tag and file names come from the script,
// the version a running frame reports comes from String(). Stamping the
// script's ingredients in must give exactly the script's full version.
func TestScriptAndPackageAgree(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	full, patch, date := script(t), script(t, "--patch"), script(t, "--date")

	if !regexp.MustCompile(`^\d+$`).MatchString(patch) {
		t.Fatalf("--patch = %q, want a count", patch)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}$`).MatchString(date) {
		t.Fatalf("--date = %q, want YYYY-MM", date)
	}

	defer stamp(t, date, patch, now())()
	if got := String(); got != full {
		t.Errorf("String() with the script's stamps = %q, script says %q", got, full)
	}

	ld := script(t, "--ldflags")
	for _, want := range []string{
		"-X github.com/chinmay28/photoframe/internal/version.Patch=" + patch,
		"-X github.com/chinmay28/photoframe/internal/version.Date=" + date,
	} {
		if !strings.Contains(ld, want) {
			t.Errorf("--ldflags = %q, missing %q", ld, want)
		}
	}
}

func script(t *testing.T, args ...string) string {
	t.Helper()
	path, err := filepath.Abs("../../scripts/version.sh")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", append([]string{path}, args...)...).Output()
	if err != nil {
		t.Fatalf("version.sh %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
