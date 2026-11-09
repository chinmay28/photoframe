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

## Unreleased

- Photos can be adjusted one by one, and the frame remembers it per photo.
  Tap **Adjust** under the photo on the Now screen, or tap a photo in
  **Up next**, to rotate it, pick its own framing (smart, fill or fit),
  zoom in up to 4×, drag it or use the alignment grid to choose what shows,
  and brighten or darken it. The editor previews the result, and saving
  re-renders the frame when that photo is on it. Adjustments live in
  `adjustments.json` in the data directory; deleting an uploaded photo
  forgets its adjustment.
- The Now screen shows the photo's file name in small text below it. Folder
  and upload sources use the file's name, Immich its original file name, and
  image URLs the last part of the URL.
- Quickstart builds `main` on the Pi by default. Downloading a prebuilt
  bundle is opt-in with `PHOTOFRAME_INSTALL=release`. `PHOTOFRAME_RELEASE`
  then defaults to `latest`, or set it to `edge` for CI's build of `main`.
- A source build is skipped when the installed version already matches
  `main`, and a run started while another is still going waits for it
  instead of building alongside it.
- Quickstart no longer reports "already installed" for an app that isn't
  running. It skips only when the app answers with the expected version.
  Otherwise it reinstalls, and shows the service log if the app still
  doesn't come up. After installing, it also prints the Pi's IP addresses,
  for phones that can't open `.local` names.

## v2026.11.6

- The frame runs without a panel attached. The app, schedule and previews
  all work, and the app says no panel is connected. The service tries the
  panel again before each refresh and draws on it after the next boot
  once one is attached. Previously it exited when the SPI device was
  missing, so installs on a Pi without its screen failed their health check.
- `install.sh` always starts the service and reminds you to reboot when it
  has just enabled SPI. Before, a first install waited for that reboot.
- Quickstart installs what is on `main` by default. CI publishes every push
  to `main` as the `edge` pre-release, and quickstart uses it when it is of
  `main`'s newest commit. Otherwise it builds `main` on the Pi.
  `PHOTOFRAME_RELEASE=latest` keeps to tagged releases.
- Fixed the changelog so this version is actually released: v2026.11.5's
  section had been inserted into the paragraph above by mistake, so no
  release was published for it.

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
