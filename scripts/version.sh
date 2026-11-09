#!/bin/sh
# The only place the app's version ingredients are computed.
#
# Scheme: vYEAR.MONTH.PATCH, a calendar version where PATCH is the commit
# count, so v2026.11.42 is the 42nd commit, built on the 2026.11 line. The
# month is not zero-padded, which keeps the string valid semver.
#
# Two ingredients, both from git:
#   Date   the month of HEAD's commit, from its committer date in the
#          committer's own timezone ("2026-11"). Not the build clock, which
#          would change the version with no commit.
#   Patch  the commit count, `git rev-list --count HEAD`.
#
# The Go binary gets both stamped in with -ldflags (see internal/version).
# The Makefile, scripts/build-release.sh, scripts/quickstart.sh and the release
# workflow all call this script, so the binary, the release tag and the file
# names always agree.
#
# Usage:
#   scripts/version.sh            # v2026.11.42
#   scripts/version.sh --patch    # 42        (value for version.Patch)
#   scripts/version.sh --date     # 2026-11   (value for version.Date)
#   scripts/version.sh --ldflags  # the -X flags for both
set -eu

cd "$(dirname "$0")/.."

PKG=github.com/chinmay28/photoframe/internal/version

# commit_month prints HEAD's commit month, or, with no commit to ask (a
# tarball, no git), the month of the build clock in UTC. The Go package falls
# back to the same clock, so a stamped and an unstamped build of the same tree
# agree on the month.
commit_month() {
  iso=$(git log -1 --format=%cI 2>/dev/null || true)
  case "$iso" in
    [0-9][0-9][0-9][0-9]-[0-9][0-9]*) printf '%s\n' "$iso" | cut -c1-7 ;;
    *) date -u +%Y-%m ;;
  esac
}

# commit_count prints the commit count on HEAD, or 0 when it can't be known:
# no repo, no git, or a shallow clone.
#
# A shallow clone is the trap. `git clone --depth 1` answers
# `rev-list --count HEAD` with 1, which isn't an error, so the build would
# quietly call itself v2026.11.1. Patch 0 is the agreed "unstamped" marker and
# visibly not a release. Clone with --filter=blob:none instead of --depth 1
# for a cheap clone that still carries the whole commit graph.
commit_count() {
  if [ "$(git rev-parse --is-shallow-repository 2>/dev/null || true)" = true ]; then
    echo "version.sh: shallow clone, reporting patch 0; run git fetch --unshallow for a real version" >&2
    echo 0
    return
  fi
  git rev-list --count HEAD 2>/dev/null || echo 0
}

full() {
  ym=$(commit_month)
  year=${ym%-*}
  month=${ym#*-}
  month=${month#0}
  printf 'v%s.%s.%s\n' "$year" "$month" "$(commit_count)"
}

case "${1:-}" in
  "") full ;;
  --patch) commit_count ;;
  --date) commit_month ;;
  --ldflags) printf -- '-X %s.Patch=%s -X %s.Date=%s\n' "$PKG" "$(commit_count)" "$PKG" "$(commit_month)" ;;
  -h|--help) sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//' ;;
  *) echo "usage: $0 [--patch|--date|--ldflags]" >&2; exit 2 ;;
esac
