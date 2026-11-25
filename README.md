# Photoframe

An e-ink photo frame for a **Raspberry Pi Zero 2 W** with a **Pimoroni Inky
Impression 7.3″** panel. One Go binary drives the panel, runs the slideshow
and serves an installable web app (PWA) where you set everything up: photo
sources, how often the photo changes, order, quiet hours, cropping, colour
tuning and the panel type.

It starts on boot, keeps showing the configured slideshow, and keeps working
offline from its cache.

## Android app

`photoframe.apk`, attached to every release, is the same app for Android
phones, plus three things a browser can't do: it finds the frame on the
Wi-Fi by itself (and opens `photoframe.local` on phones that can't), it
shows up in your gallery's **Share** menu to send photos straight to the
frame (HEIC included), and a long press on its icon gives the frame's next,
previous and pause buttons. It is a thin client: the Pi drives the panel.

Install it from
<https://github.com/chinmay28/photoframe/releases/latest/download/photoframe.apk>.
How to use it is in [docs/android.md](docs/android.md), how it is built in
[apps/android/README.md](apps/android/README.md).

## Hardware

- Raspberry Pi Zero 2 W running Raspberry Pi OS **Bookworm or later**. The
  64-bit Lite image is recommended.
- Inky Impression 7.3″. Both variants are supported: the 2025 **Spectra 6**
  edition (the default) and the original **7-colour ACeP**. Choose yours in
  the app under *Device → Panel*.
- The HAT's A–D buttons: **A** next photo, **B** previous photo, **C**
  pause/resume, **D** shows the app's address on the frame.

## Quick start on the Pi

Flash Raspberry Pi OS Lite with Raspberry Pi Imager, setting Wi-Fi, SSH and
the hostname `photoframe` in the Imager's settings. Then, on the Pi:

```sh
curl -fsSL https://raw.githubusercontent.com/chinmay28/photoframe/main/scripts/quickstart.sh | sudo bash
```

It installs what is on `main`: it clones the repository, builds `main` on the
Pi (installing Go first if needed) and installs it. Reboot once if it says so,
then open **http://photoframe.local** on your phone.

The first build on a Pi Zero 2 W takes a while. Go's caches are kept in
`/var/cache/photoframe-build`, so later builds take about a minute. If a run
is cut off, running it again picks up where it stopped, or waits for a build
that is still going. When the installed version already matches `main`,
nothing is rebuilt.

Installing a prebuilt bundle instead is opt-in. It takes seconds and nothing
is compiled on the Pi:

```sh
# the newest tagged release
curl -fsSL …/quickstart.sh | sudo PHOTOFRAME_INSTALL=release bash
# CI's build of main's newest commit
curl -fsSL …/quickstart.sh | sudo PHOTOFRAME_INSTALL=release PHOTOFRAME_RELEASE=edge bash
```

Run the same command again to upgrade. Before an upgrade it keeps the running
binary and a copy of `settings.json` (in `/var/lib/photoframe/backups`). If
the new version doesn't answer within a minute, it puts both back and restarts
the old version. Photos and settings in `/var/lib/photoframe` are never
touched.

To remove it, keeping photos and settings:

```sh
curl -fsSL https://raw.githubusercontent.com/chinmay28/photoframe/main/scripts/quickstart.sh | sudo bash -s -- --uninstall
```

Options are environment variables, set after `sudo`:

| Variable | |
| --- | --- |
| `PHOTOFRAME_INSTALL` | `source` (default: build `main` on the Pi), `release` (download a prebuilt bundle) or `auto` (a release if there is one, else source) |
| `PHOTOFRAME_RELEASE` | with `release`: `latest` (default), `edge` (CI's build of `main`) or a tag |
| `PHOTOFRAME_REF` | branch, tag or commit to build in source mode (default `main`) |
| `PHOTOFRAME_BUNDLE` | a release `.tar.gz` already on the Pi, installed without going online |
| `PHOTOFRAME_HOSTNAME` | set the Pi's hostname, so the app is at `http://<name>.local` |
| `PHOTOFRAME_PHOTOS_DIR` | folder for photos you copy onto the Pi yourself (default `/var/photoframe`, or the one set before); see [Photo sources](#photo-sources) |
| `PHOTOFRAME_FORCE=1` | reinstall even if that version is already running |
| `PHOTOFRAME_PURGE=1` | with `--uninstall`, also delete `/var/lib/photoframe` and the service user |

For example, `curl -fsSL …/quickstart.sh | sudo PHOTOFRAME_REF=my-branch bash`
builds a branch.
Running `sudo ./scripts/quickstart.sh` from a checkout on the Pi builds that
checkout as it stands, which is how to try uncommitted work. Everything it
prints is also logged to `/var/log/photoframe-quickstart.log`.

## Install from your computer

1. Flash Raspberry Pi OS Lite as above.
2. On your computer, with Go 1.26.4 or later installed:

   ```sh
   make deploy PI=pi@photoframe.local
   ```

   This cross-compiles the binary, copies it to the Pi and runs
   `deploy/install.sh`, which:
   - installs `/usr/local/bin/photoframe`
   - creates a `photoframe` system user in the `spi` and `gpio` groups
   - adds `dtparam=spi=on` and `dtoverlay=spi0-0cs` to `config.txt` if they
     are missing. The driver drives chip-select itself on GPIO 8, as
     Pimoroni's library does.
   - creates the photos folder, `/var/photoframe` unless `--photos-dir`
     names another, gives it to that user and opens it to everyone
   - installs and enables `photoframe.service`
   - announces the app over mDNS (`_photoframe._tcp`, an Avahi service file),
     so the Android app can find the frame
3. If the installer says so, reboot once: `sudo reboot`.
4. On your phone, open **http://photoframe.local** and use *Add to Home
   Screen* to install the app.

Until you add photos, the frame shows a welcome screen with the app's address.

You can install before the screen is attached. The app, sources and schedule
all work, and the Now screen shows the preview marked "no panel". Power the Pi
off, attach the Inky Impression and boot. The current photo is drawn as soon as
the service starts.

To install by hand instead, copy `dist/photoframe-linux-arm64` and the two
files in `deploy/` to the Pi, then run
`sudo ./install.sh --binary ./photoframe-linux-arm64`.

Useful commands:

```sh
journalctl -u photoframe -f      # logs
sudo systemctl restart photoframe
```

## Photo sources

| Type | What it does |
| --- | --- |
| Upload | Photos uploaded from the app, stored on the SD card. |
| Folder | Any folder on the Pi, scanned recursively. Mount a NAS share (SMB/NFS via `/etc/fstab`) and point a folder source at it. Sub-folder names are used as captions. |
| Immich | One Immich album. Uses Immich's preview-size images (about 1440 px), which are cached on the SD card. Captions use Immich's place names. |
| Image URL | A single image URL, re-fetched on every sync with conditional requests. Good for "picture of the day" feeds. |

Uploads, folders and image URLs can be JPEG, PNG, GIF, WebP or HEIC/HEIF
(`.heic`, `.heif`, as iPhones and many Android phones save photos). HEIC is
decoded on the Pi in pure Go ([gen2brain/h265](https://github.com/gen2brain/h265)),
so nothing extra needs installing. It decodes about three times slower than
JPEG, which can add seconds per photo on a Pi Zero 2 W before the panel
starts its refresh. The Android app still converts HEIC to a smaller JPEG
when you share. A HEIC photo is turned upright by the rotation stored in the
file, and dated by its EXIF.

Photos in upload and folder sources are given mode `644` and, where the
service is allowed to, handed to the user it runs as (`photoframe`): uploads
as they are saved, and anything else found in the folder, however it got
there, at the next sync. Only image files are changed; a photo that can't be
changed is shown as it is. One the service may not even read is left out,
and the source's line under *Sources* says how many and why.

For photos you copy onto the Pi yourself, the installer makes
`/var/photoframe`: add it in the app as a folder source. It is open to
every user on the Pi (mode `777`, sub-folders included), so photos can be
copied in, replaced and removed without `sudo`, for example
`scp *.jpg pi@photoframe.local:/var/photoframe/`. Every install or upgrade
gives the folder and everything in it to the `photoframe` user
(`chown -R photoframe:photoframe`), and the service may write there, so each
sync also takes over photos copied in since, whoever copied them. Running
the quickstart again does the same by hand, even when there is nothing to
upgrade, and has the app rescan. To use another folder, name it once; later runs
keep it:

```sh
curl -fsSL …/quickstart.sh | sudo PHOTOFRAME_PHOTOS_DIR=/srv/photos bash
```

(`sudo ./install.sh --photos-dir /srv/photos` when installing by hand.) A
folder source anywhere else is still shown, but is read-only to the service,
so its photos keep the mode and owner they have.

Taking over other users' files is why the service's unit has `CAP_CHOWN`.
To keep that from reaching anything else, the unit lets the service open no
devices except the panel's SPI and GPIO.

Sources sync at startup, every 15 minutes (`-sync-interval`), and on demand
from the app. If a remote source can't be reached, its cached photos stay in
rotation.

## How it works

```
cmd/photoframe       wiring, flags, signal handling
internal/config      settings types, validation, atomic JSON store with change notifications
internal/adjust      per-photo rotation, framing, zoom, alignment and brightness, stored by photo ID
internal/source      Dir (uploads/folders), Immich, URL image sources
internal/library     merges enabled sources into one catalogue; keeps cached photos on errors
internal/playlist    next/previous: shuffle deck (no repeats per cycle), newest, oldest, on-this-day;
                     play-next and move-to-back queueing, and jumping to a chosen photo
internal/schedule    when the next change is due, with quiet hours (including windows past midnight)
internal/imagefmt    which image files are read (JPEG, PNG, GIF, WebP, HEIC); registers the decoders
internal/render      decode + EXIF orientation, smart/fill/fit with zoom and alignment, saturation/contrast,
                     Floyd–Steinberg / Atkinson / ordered dithering, captions, info screens
internal/inky        panel driver over spidev + GPIO character device (no cgo)
internal/player      the slideshow loop: timer, commands, retries, state across restarts
internal/api         JSON API + embedded PWA; same-origin check on every write
internal/buttons     A–D buttons with debounce
internal/sysinfo     temperature, uptime, Wi-Fi signal for the Device screen
internal/version     vYEAR.MONTH.PATCH; the ingredients are stamped at link time
web/                 the PWA (plain ES modules, no build step)
apps/android/        the Android app: a thin client around the PWA (see its README)
cmd/photoframe-icons the Android icons, derived from web/icons/icon.svg (make icons)
deploy/              install.sh (which also announces the app over mDNS) and the systemd unit
scripts/
  quickstart.sh      one-command install / upgrade / rollback / uninstall on the Pi
  version.sh         the only place the version ingredients are computed
  build-release.sh   the Pi release bundles + SHA256SUMS
```

Design choices:

- **No refresh on restart if nothing is due.** E-ink keeps its image without
  power. After a reboot the frame waits until the next photo is actually due
  instead of flashing a new one straight away.
- **Panel errors are retried.** If the panel fails, the same photo is resent
  after a minute. A photo that won't decode is skipped.
- **Quiet hours** only stop automatic changes. The buttons and the app still
  work during them.
- The panel driver is a port of Pimoroni's MIT-licensed
  [inky](https://github.com/pimoroni/inky) Python library (`inky_e673.py` and
  `inky_ac073tc1a.py`). Tests check the protocol against a fake panel.

### API

All responses are JSON unless noted.

| Method & path | |
| --- | --- |
| `GET /api/status?upNext=4` | what's on screen, next change, queue |
| `POST /api/control/{next,prev,pause,resume,refresh,clear,info}` | commands |
| `GET /api/preview.png` | the last image sent to the panel, in the panel's colours |
| `GET/PUT /api/settings` | all settings except sources |
| `GET/POST /api/sources`, `PUT/DELETE /api/sources/{id}` | sources (API keys are never returned) |
| `POST /api/sources/{id}/sync` | sync now |
| `POST /api/sources/{id}/photos` | multipart upload (field `photos`) into an upload source |
| `GET /api/photos?source=id`, `GET /api/photos/{id}/thumb`, `DELETE /api/photos/{id}` | photos |
| `GET /api/photos/{id}/image` | a larger rendition (1280 px) for the editor |
| `GET /api/queue` | every photo: the one on screen, what's coming up in order, and the rest |
| `POST /api/photos/{id}/show` | put a photo on the frame now; the slideshow carries on from it |
| `POST /api/photos/{id}/queue` | `{"position":"front"}` plays it next, `"back"` after every other photo |
| `GET/PUT/DELETE /api/photos/{id}/adjustment` | a photo's own rotation, framing, zoom, position and brightness; re-renders it if on screen |
| `GET /api/device`, `POST /api/device/restart` | device info, restart service |

The app has no login and is meant for your home network only. Requests
that change anything must come from the app's own origin. An Immich
source's stored API key is only reused when the server URL stays the same.

## Development

```sh
make test     # unit tests, with the race detector
make dev      # run on a laptop; frames go to devdata/display.png
make version  # the version this tree builds as
make release  # Pi release bundles in dist/
make android  # the Android app (needs a JDK 17+ and the Android SDK)
make icons    # redraw the Android icons from web/icons/icon.svg
```

Open http://localhost:8080. The PNG display simulates a 2-second refresh
(`-png-delay`).

Flags: `-data`, `-listen`, `-display inky|png`, `-png`, `-spi`
(default `/dev/spidev0.0`), `-gpiochip` (default `gpiochip0`), `-buttons`,
`-sync-interval`, `-version`.

## Versions and releases

Versions are calendar-based: `vYEAR.MONTH.PATCH`, for example `v2026.11.42`.
Nobody picks them; they are made of two ingredients from git:

- **Date**: the month of the commit being built, from its committer date. The
  line moves when a commit lands in a new month, and rebuilding an old commit
  gives the name it shipped with.
- **Patch**: the repository's commit count, so every commit is a patch release.

A binary can't ask git, so `scripts/version.sh` computes both and the build
stamps them in with `-ldflags -X` (`internal/version.Patch` and
`internal/version.Date`). `make build`, `make pi`, `make release`,
`scripts/quickstart.sh` and the release workflow all go through it, and a test
checks that the script and the Go package render the same string.

```sh
make version                 # v2026.11.42
scripts/version.sh --patch   # 42
scripts/version.sh --date    # 2026-11
```

A bare `go build` stamps nothing and builds patch 0, a development build,
which the app labels as such. A shallow clone (`--depth 1`) has no real commit
count, so it also builds patch 0; clone with `--filter=blob:none` instead.

The version shows in `photoframe -version`, the startup log, `/api/device`
and the About card at the bottom of the app's Device screen, next to the
developer badge. The Android app is stamped with the same version
(`-PappVersionName`, and the commit count as `-PappVersionCode`).

`make release` builds `dist/photoframe-<version>-linux-{arm64,armv7}.tar.gz`
and `SHA256SUMS`. Each tarball holds the binary, `install.sh` and the unit.
The release workflow adds the Android app as `photoframe.apk` (a fixed
name, so `releases/latest/download/photoframe.apk` always works), stamped
with the same version as its name and the commit count as its version code,
and lists it in `SHA256SUMS` too.
`.github/workflows/release.yml` runs on every push to `main`:

- It always publishes the bundles as the `edge` pre-release, replacing the
  previous one. The quick start installs it with
  `PHOTOFRAME_INSTALL=release PHOTOFRAME_RELEASE=edge`. Being a
  pre-release, it never becomes GitHub's "latest release".
- When the commit has a `## <its version>` section in `CHANGELOG.md`, it also
  publishes a tagged release. A matching tag or a manual dispatch does the
  same. `PHOTOFRAME_INSTALL=release` installs the newest of those.

## Not yet supported

- iCloud and Google Photos shared albums, which have no stable public API.
  Use Immich, a folder, or uploads instead.
