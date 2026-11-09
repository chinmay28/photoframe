# Photoframe

An e-ink photo frame for a **Raspberry Pi Zero 2 W** with a **Pimoroni Inky
Impression 7.3″** panel. One Go binary drives the panel, runs the slideshow
and serves an installable web app (PWA) where you set everything up: photo
sources, how often the photo changes, order, quiet hours, cropping, colour
tuning and the panel type.

It starts on boot, keeps showing the configured slideshow, and keeps working
offline from its cache.

## Hardware

- Raspberry Pi Zero 2 W running Raspberry Pi OS **Bookworm or later**. The
  64-bit Lite image is recommended.
- Inky Impression 7.3″. Both variants are supported: the 2025 **Spectra 6**
  edition (the default) and the original **7-colour ACeP**. Choose yours in
  the app under *Device → Panel*.
- The HAT's A–D buttons: **A** next photo, **B** previous photo, **C**
  pause/resume, **D** shows the app's address on the frame.

## Install on the Pi

1. Flash Raspberry Pi OS Lite with Raspberry Pi Imager. Set Wi-Fi, SSH and the
   hostname `photoframe` in the Imager's settings.
2. On your computer, with Go 1.24 or later installed:

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
   - installs and enables `photoframe.service`
3. If the installer says so, reboot once: `sudo reboot`.
4. On your phone, open **http://photoframe.local** and use *Add to Home
   Screen* to install the app.

Until you add photos, the frame shows a welcome screen with the app's address.

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

Sources sync at startup, every 15 minutes (`-sync-interval`), and on demand
from the app. If a remote source can't be reached, its cached photos stay in
rotation.

## How it works

```
cmd/photoframe       wiring, flags, signal handling
internal/config      settings types, validation, atomic JSON store with change notifications
internal/source      Dir (uploads/folders), Immich, URL image sources
internal/library     merges enabled sources into one catalogue; keeps cached photos on errors
internal/playlist    next/previous: shuffle deck (no repeats per cycle), newest, oldest, on-this-day
internal/schedule    when the next change is due, with quiet hours (including windows past midnight)
internal/render      decode + EXIF orientation, smart/fill/fit, saturation/contrast,
                     Floyd–Steinberg / Atkinson / ordered dithering, captions, info screens
internal/inky        panel driver over spidev + GPIO character device (no cgo)
internal/player      the slideshow loop: timer, commands, retries, state across restarts
internal/api         JSON API + embedded PWA; same-origin check on every write
internal/buttons     A–D buttons with debounce
internal/sysinfo     temperature, uptime, Wi-Fi signal for the Device screen
web/                 the PWA (plain ES modules, no build step)
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
| `GET /api/device`, `POST /api/device/restart` | device info, restart service |

The app has no login and is meant for your home network only. Requests
that change anything must come from the app's own origin. An Immich
source's stored API key is only reused when the server URL stays the same.

## Development

```sh
make test     # unit tests, with the race detector
make dev      # run on a laptop; frames go to devdata/display.png
```

Open http://localhost:8080. The PNG display simulates a 2-second refresh
(`-png-delay`).

Flags: `-data`, `-listen`, `-display inky|png`, `-png`, `-spi`
(default `/dev/spidev0.0`), `-gpiochip` (default `gpiochip0`), `-buttons`,
`-sync-interval`, `-version`.

## Not yet supported

- HEIC files. iOS converts photos to JPEG when you pick them in the upload
  dialog.
- iCloud and Google Photos shared albums, which have no stable public API.
  Use Immich, a folder, or uploads instead.
