# Photoframe for Android — design

The Android app is the frame's own web app in a web view, with a few native
pieces where Android does a job better than a page can. Using it is in the
[user guide](../../docs/android.md). It is modelled on SAND Vault for Android
(SandHarbor's `apps/android`) and keeps its layout and rules, minus what a
thin client of a panel driver doesn't need.

```
 ┌─────────────────────────── the phone ────────────────────────────┐
 │ MainActivity  WebView ── the frame's web app, from Origin only    │
 │                  ▲ photoframe-bridge.js → window.photoframeApp    │
 │ FrameResolver ── photoframe.local → system resolver ‖ NSD         │
 │ ShareActivity ── share sheet → (HEIC → JPEG) → multipart upload   │
 │ ControlActivity ─ launcher shortcut → POST /api/control/{cmd}     │
 │ SetupActivity / SettingsActivity ── address, Find, version, badge │
 └──────────────────────────────┬────────────────────────────────────┘
                                ▼  http://photoframe.local (port 80)
     the Pi: photoframe (Go) ── panel, slideshow, photos, settings
             avahi-daemon ──── _photoframe._tcp (deploy/install.sh)
```

## Thin client only

The frame's server drives the panel's hardware over SPI and GPIO on the Pi
and reads its A–D buttons, so there is no "run it on the phone" mode: a phone
has no panel to drive. The app carries no Go and no native libraries (the
Android workflow fails if an APK grows any), and every setting other than
the frame's address lives on the frame.

## Native where it pays

**Discovery and `.local` names.** The frame is installed as
`photoframe.local`, but many Android versions resolve no `.local` names, and
the web view uses the phone's resolver. NSD (Android's mDNS) always can, so:

- `deploy/install.sh` drops an Avahi service file announcing
  `_photoframe._tcp`, named after the Pi's hostname (`%h`), on the port the
  unit listens on. Avahi already runs on Raspberry Pi OS and already answers
  for `<hostname>.local`; nothing runs in `photoframe` for it, which keeps the
  Pi Zero 2 W's load where it was. `quickstart.sh --uninstall` removes it.
- **Find on this network** lists those services (`FrameDiscovery`, NSD;
  `core/Discovery` reads the answers). Picking one stores its `.local` name,
  not its IP, so a new DHCP lease doesn't strand the app.
- On every launch `FrameResolver` races the phone's own resolver against NSD
  for a `.local` address. If only NSD knows it, the web view opens the
  address NSD found for that session. The bridge and the navigation rules are
  keyed on that origin.

**Share to Photoframe.** `ShareActivity` takes `ACTION_SEND` /
`SEND_MULTIPLE` of `image/*` and posts the photos to the frame's existing
`POST /api/sources/{id}/photos`, to the same upload source the page's Upload
button picks (`core/FrameApi.uploadSource`), in one streamed multipart
request, so the frame rescans once. No server change was needed. What a
browser can't do is done first: HEIC (and any other image the frame can't
decode) becomes a JPEG through `ImageDecoder`, capped at 4096 px
(`core/UploadPlan`), and every name ends in the extension the frame accepts
it by. `core/Multipart` writes the body; its test must reproduce
`internal/api/testdata/android-upload.multipart` byte for byte, and
`internal/api/android_test.go` posts that file to the real handler — so the
two sides can't drift. Only `content:` addresses are read
(`core/ShareInput`): a `file:` one would let the sharing app pick a path in
this app's private storage.

**Launcher shortcuts.** Next photo, Previous and Pause / resume are the
frame's A, B and C buttons (`POST /api/control/{next,prev,pause,resume}`);
pause-or-resume reads `/api/status` first (`core/Control`). They run in
`ControlActivity`, which shows only a toast, and is not exported: a launcher
starts a shortcut as this app. `FrameApiTest` checks `shortcuts.xml` names
exactly `core/Control`'s actions. A home-screen widget was left out: the
panel redraw takes long enough that a widget showing the current photo would
mostly be wrong, and the shortcuts already put the buttons a tap away.

Not built: downloads and `navigator.share` handling (SAND Vault's
`Transfers`, `Downloads`, `Sharing`), because the frame's page downloads and
shares nothing; `photoframe-bridge.js` only defines `window.photoframeApp`
(`hello`, `openSettings`). The page doesn't look for it yet: the app's
settings are a long press on the icon away, and on the error screen.

## The shell

- `MainActivity` is origin-locked (`core/NavigationPolicy`): the frame's own
  pages stay in the app, other `http(s)` goes to the browser, `intent:` is
  launched only with its component and selector stripped and limited to apps
  that take browser links, `javascript:`/`file:`/`content:` are refused.
  `<input type=file>` goes to Android's picker (`core/AcceptTypes`).
- The bridge script and the `PhotoframeAndroid` listener are given to that
  origin alone (`addDocumentStartJavaScript` and `addWebMessageListener` with
  it as the only rule).
- Cleartext is allowed and user CAs trusted (`network_security_config.xml`):
  a frame on a home network is plain HTTP.
- Edge to edge; the root view takes the system-bar, cutout and keyboard
  insets. Back goes back in the web view's history.

## The developer badge

`dev_badge.png` and `dev_badge_full.png` are the web app's own
(`web/icons/`), copied by the icon generator. The setup screen carries the
small one (36dp in a 44dp round target), the settings screen a round cut of
it; a tap throws the big one up full screen with `github.com/chinmay28` in
monospace under it (`DevBadge`), on the same 3 s timeline as the web app's
CSS (`core/DevFlash`): the veil fades in over 7% and out from 82%, the badge
lands from 0.82 with an overshoot. With animations off in Android's
settings it shows still, as `prefers-reduced-motion` does on the web.

## Icons

Every icon is derived from `web/icons/icon.svg` by `make icons`
(`cmd/photoframe-icons`): the SVG is drawn fresh at each size with
`golang.org/x/image/vector`, so nothing is scaled from a bitmap.

| File | What |
|---|---|
| `mipmap-*/ic_launcher.png` | the whole rounded tile, 48dp per density |
| `mipmap-*/ic_launcher_foreground.png` | the scene without its tile on a 108dp canvas, the artwork's square in the middle 72dp |
| `mipmap-anydpi-v26/ic_launcher.xml`, `values/ic_launcher_background.xml`, `mipmap-*/ic_launcher_monochrome.png` | the adaptive icon, with a themed-icon silhouette (the light parts of the scene), on the tile's colour (`#2A2724`) |
| `drawable-nodpi/photoframe_logo.png` | the mark on the setup and connecting screens |
| `drawable-nodpi/dev_badge*.png` | the developer badge, and its round cut |

The generator reads only the SVG features the artwork uses (rect, circle,
paths of M/L/Q/Z, flat fills) and refuses anything else, so a richer redraw
fails loudly. `go test ./cmd/photoframe-icons` fails when a committed copy has
drifted from the source, and when any `@drawable/` or `@mipmap/` reference in
the app names a file that doesn't exist. Never edit a derived file by hand.
The app posts no notifications, so there is no status-bar icon. The four
shortcut icons are hand-drawn vectors (`drawable/ic_shortcut_*.xml`).

## Version

`versionName` is the frame's own version (`scripts/version.sh`,
`vYEAR.MONTH.PATCH` from the commit), and `versionCode` the commit count,
which only grows: `-PappVersionName` / `-PappVersionCode`, stamped by
`make android` and the release workflow. Unstamped, it is `dev` / 1. The
setup screen shows it under **Continue**, the settings under *Version*.

## Code layout and checks

```
apps/android/
├── core/   plain Kotlin, JVM-tested, no Android:
│           Origin, NavigationPolicy, AcceptTypes, AppConfig (+ ShareInput),
│           Discovery, Bridge, FrameApi (+ Control), Uploads (UploadPlan,
│           Multipart), FileNames, DevFlash
└── app/    the Android shell around it:
            MainActivity, PageBridge + assets/photoframe-bridge.js,
            FrameResolver, FrameDiscovery, FrameHttp, ShareActivity,
            ControlActivity, SetupActivity, SettingsActivity, DevBadge,
            ConfigStore
```

```sh
cd apps/android
./gradlew -PcoreOnly :core:test                  # no Android SDK needed
./gradlew :core:test :app:lintDebug :app:assembleRelease
make android                                      # from the root: stamped APK
```

Some `:core` tests read files outside it on purpose: `deploy/install.sh`
(the announced service type), `shortcuts.xml` (the shortcut actions),
`assets/photoframe-bridge.js` (the messages) and the upload fixture.

The release build is signed with the debug key so it sideloads as it
stands; anybody publishing to a store signs it with their own. There is no
emulator in CI, so nothing launches the app there: `.github/workflows/android.yml`
runs the tests, lint and the build, and checks the APK carries the bridge,
the shortcuts and no native code.
