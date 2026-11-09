// Package version carries the application version.
//
// The scheme is calendar-based: vYEAR.MONTH.PATCH, where the patch number is
// the repository's commit count. Every commit is a patch release, so
// v2026.11.42 is the 42nd commit, built on the 2026.11 line. The month is not
// zero-padded, which keeps the string valid semver (semver forbids a leading
// zero).
//
// The version is made of two ingredients, both taken from git and neither
// chosen by a person:
//
//   - Date: the month of the commit being built, from its committer date in
//     the committer's own timezone. The line moves as soon as a commit lands
//     in a new month, and a rebuild of an old commit keeps the name it shipped
//     with, because a commit's date never changes.
//   - Patch: the commit count, `git rev-list --count HEAD`.
//
// A compiled binary can't ask git, so both are stamped in at link time:
//
//	go build -ldflags "-X github.com/chinmay28/photoframe/internal/version.Patch=$(scripts/version.sh --patch) \
//	                   -X github.com/chinmay28/photoframe/internal/version.Date=$(scripts/version.sh --date)"
//
// `make build`, `make pi`, scripts/build-release.sh and scripts/quickstart.sh
// all do this through scripts/version.sh, the only place the ingredients are
// computed.
//
// A bare `go build` stamps nothing and is a development build: patch 0, which
// is never a release, under the month of the build clock. That is the only
// time the clock is used, because an unstamped build has no commit to ask.
package version

import (
	"strconv"
	"time"
)

// Patch is the repository's commit count, stamped at link time. A bare
// `go build` leaves it at "0": patch 0 marks an unstamped development build,
// never a release.
var Patch = "0"

// Date is the month of the commit being built, as "YYYY-MM", stamped at link
// time beside Patch. It is empty on an unstamped build, which then reports
// the month it was compiled in.
var Date = ""

// now is the clock an unstamped build falls back to. It is a variable so a
// test can hold it still.
var now = time.Now

// Year and Month are the release line: the commit's month when stamped, the
// build's when not. Month is a calendar month, 1–12.
func Year() int  { y, _ := yearMonth(); return y }
func Month() int { _, m := yearMonth(); return m }

// Dev reports whether this is an unstamped development build.
func Dev() bool { return Patch == "0" }

func yearMonth() (int, int) {
	if t, err := time.Parse("2006-01", Date); err == nil {
		return t.Year(), int(t.Month())
	}
	t := now().UTC()
	return t.Year(), int(t.Month())
}

// String renders the full version with a `v` prefix, matching the release
// tags (v2026.11.42). This is the only rendering: the -version flag, the
// startup log line, /api/device and the app's Device screen all show it.
func String() string {
	y, m := yearMonth()
	return "v" + strconv.Itoa(y) + "." + strconv.Itoa(m) + "." + Patch
}
