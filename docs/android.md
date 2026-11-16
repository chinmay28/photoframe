# Photoframe on Android

The Photoframe app puts your frame's app on an Android phone, with a few
things a browser can't do: it finds the frame on the Wi-Fi by itself, it
appears in the share sheet of your gallery so you can send photos straight to
the frame, and a long press on its icon gives you the frame's next, previous
and pause buttons.

It is a **thin client**. The frame does all the work: the Pi runs the
slideshow, keeps the photos and drives the panel, and the app opens the
frame's own web app, the same one you get at **http://photoframe.local** in a
browser. So everything the web app does, the phone app does — sources, the
schedule, quiet hours, cropping and per-photo adjustments, colour tuning, the
panel type — and the frame has to be on and on the same network as the phone.
How the app works inside is in [its design notes](../apps/android/README.md).

## Install the app

Every [release](https://github.com/chinmay28/photoframe/releases) attaches it
as `photoframe.apk`, and the latest is always at:

```
https://github.com/chinmay28/photoframe/releases/latest/download/photoframe.apk
```

Open that address on the phone, download the APK, and open it. Android asks
once to let your browser install apps; allow it. A newer release installs
over the old one and keeps your settings. The `edge` pre-release (CI's build
of `main`) carries the app too.

It needs Android 10 or newer.

## The first screen

The app asks which frame to open, and starts on `photoframe.local`, the name
the [quick start](../README.md#quick-start-on-the-pi) installs a frame under.
With one frame at home, tap **Continue**.

- **Find on this network** lists every frame that announces itself on your
  Wi-Fi, by name. Tap one to use it. A frame announces itself once it has been
  installed or upgraded with this version's installer (it adds an mDNS
  announcement to Avahi, which Raspberry Pi OS already runs). An older frame
  doesn't show up here; run the quick start on it again, or type its address.
- **Or type the address**: `photoframe.local`, `kitchen.local`, or an IP
  address such as `192.168.1.20`. Press button **D** on the frame to see its
  address. An address with no `http://` means `http://` on port 80, which is
  where the frame's app is.

Many Android phones can't open `.local` names by themselves, which is why
`http://photoframe.local` may not work in the phone's browser. The app asks
the network for the frame instead (the same mDNS announcement *Find* uses), so
`photoframe.local` works in the app either way — and keeps working when the
Pi's IP address changes.

The app's version is in small print under **Continue**, and the developer
badge sits at the bottom of the screen: tap it.

## Using it

The app shows the frame's own app, full screen. Back goes back a screen.
Uploading from the **Sources** screen opens Android's file picker.

If the frame can't be reached, the app says so, with **Retry** and **App
settings**. Check that the phone is on the frame's Wi-Fi and that the frame is
on.

### Send photos from your gallery

In Google Photos, the Gallery, Files or any app that shares photos, pick one
or more photos, tap **Share**, then **Photoframe**. The app sends them to the
frame's upload source (*Uploaded from phones*, unless you renamed it — the
same place the Upload button on the Sources screen puts them) and tells you
how many arrived. They join the slideshow straight away.

Photos the frame can't read itself are converted on the phone first: HEIC,
what many phone cameras save, becomes a JPEG (at most 4096 pixels on its long
side, plenty for the panel and for zooming in the editor). Videos and other
files are left out, and the app says which.

If the frame has no upload source, add one under **Sources** in the app.

### The frame's buttons on your home screen

Long-press the app's icon:

| Shortcut | Does what the frame's button does |
|---|---|
| **Next photo** | A: the next photo |
| **Previous** | B: the previous photo |
| **Pause / resume** | C: pause or resume the slideshow |
| **Settings** | the app's own settings |

Each sends the command without opening the app and confirms with a short
message; the panel then redraws as it does for the button. Drag one onto the home
screen to keep it there.

## App settings

Behind a long press on the app's icon, or **App settings** when the frame
can't be reached:

| Setting | What it does |
|---|---|
| Frame address | Which frame the app opens. |
| Find frames on this network | The same list as on the first screen. |
| Version | The app's version, the same `vYEAR.MONTH.PATCH` as the frame's. |
| Developer | The developer badge. |

Everything about the frame itself is in the frame's app.

## What it doesn't do

- **Run without the frame.** The app has no copy of your photos or settings;
  they live on the frame.
- **Reach the frame from outside your home network.** The frame's app has no
  login and is meant for your home network only. Use a VPN such as Tailscale
  to reach it from elsewhere, and type the address it gives the Pi.
- **HEIC from the Sources screen's Upload button.** That button is the web
  app's own, so the file picker offers what the frame can read. Share HEIC
  photos from your gallery instead, and the app converts them.
