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

- The app is much faster and steadier on a Pi Zero 2 W.
  - Thumbnails and the editor's image are made about 3.5× and 6× faster
    (a 12 MP JPEG, measured on a laptop), and loading a photo for the panel
    about 2× faster: JPEGs are shrunk by a whole factor before the quality
    scaler sees them.
  - Thumbnails are made ahead of time, one at a time, whenever photos are
    added. The frame never decodes more than two photos at once for the
    app, and several requests for one photo share the work. Before, every
    redraw of a screen could start the same thumbnail again.
  - The app redraws only the part of a screen that changed, so previews and
    thumbnails no longer reload every few seconds, and it waits for one
    answer before asking again.
  - A missed request no longer blanks the screen with "Failed to fetch".
    Reads are retried, the screen keeps what the frame last said and shows
    "Reconnecting…", and a preview that failed to load is asked for again.
  - The preview survives a restart of the service (`preview.png` in the
    data directory). It used to stay a broken image until the next photo.
  - The app opens from the phone's own copy of its files and of the last
    answers, then catches up, and reloads itself once when the frame's
    software has changed. Unchanged answers cost a `304` instead of a body.
  - The editor opens with the thumbnail and sharpens when the larger image
    arrives, instead of a blank canvas.
  - The service runs with `GOMEMLIMIT=256MiB`, to stay clear of the Pi's
    512 MB.
- Tapping a photo in **Up next** opens a menu instead of acting straight
  away: show it on the frame now, play it next, move it to the back of the
  queue, adjust it, or delete it (uploaded photos only). **View all** on
  the Now screen lists every photo in the order it will play, with the same
  menu. Play next and move to back work in every order; in newest, oldest
  and on-this-day the order keeps its place around them. Queue changes are
  kept in memory, so a restart goes back to the normal order.
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
