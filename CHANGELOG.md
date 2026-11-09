# Changelog

Versions are `vYEAR.MONTH.PATCH`: the month of the commit and the repository's
commit count (see `internal/version`). `make version` prints the version the
current tree builds as.

A section headed with a version, such as `## v2026.11.40`, is what makes that
commit a release: when the commit that builds that version lands on `main`,
`.github/workflows/release.yml` publishes the Pi bundles under that tag with
the section as the release notes. To cut a release, add the section in the
commit that will be that version. With squash merges, that is the merge
commit: one more than `main`'s current count.

## v2026.11.4

- First published release, so `scripts/quickstart.sh` installs a prebuilt
  bundle in seconds instead of compiling on the Pi.
- Quickstart's source build now works on a Pi Zero 2 W: it builds on disk
  rather than the RAM-backed `/tmp`, adds temporary swap when memory is
  short, keeps Go's caches so an interrupted build resumes on the next run,
  and prints progress so long compiles don't look hung.
- One-command install on the Pi: `scripts/quickstart.sh`, with upgrade in
  place, a health check that rolls back a bad upgrade, and `--uninstall`.
- Calendar versions, `vYEAR.MONTH.PATCH`, stamped from git at build time.
- Release bundles for 64-bit and 32-bit Raspberry Pi OS.
- Developer badge on the app's Device screen.
