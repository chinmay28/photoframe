#!/usr/bin/env bash
# Install or upgrade photoframe on a Raspberry Pi (Raspberry Pi OS Bookworm
# or later). Run on the Pi, as root, from a directory containing the binary
# and photoframe.service:
#
#   sudo ./install.sh [--binary ./photoframe-linux-arm64] [--hostname photoframe]
#                     [--photos-dir /var/pictures]
#
# --photos-dir is the folder for photos you copy onto the Pi yourself. It is
# created if missing, given with everything in it to the photoframe user, and
# made writable to the service and, with its sub-folders, to every user.
# Without the option, the folder chosen by an earlier run is kept; the first
# time, it is /var/pictures.
#
# Safe to re-run: it only changes what is missing.
set -euo pipefail

BIN="./photoframe"
NEW_HOSTNAME=""
PHOTOS_DIR=""
DEFAULT_PHOTOS_DIR=/var/pictures
# Where a folder other than the default is remembered: a drop-in that lets
# the service write to it, as the unit itself does for the default.
PHOTOS_DROPIN=/etc/systemd/system/photoframe.service.d/photos-dir.conf
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BIN="$2"; shift 2 ;;
    --hostname) NEW_HOSTNAME="$2"; shift 2 ;;
    --photos-dir) PHOTOS_DIR="$2"; shift 2 ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

HERE="$(cd "$(dirname "$0")" && pwd)"
[[ $(id -u) -eq 0 ]] || { echo "Run as root: sudo $0 $*" >&2; exit 1; }
[[ -x "$BIN" ]] || { echo "Binary not found or not executable: $BIN" >&2; exit 1; }

# Settle the photos folder before changing anything. It is handed over
# recursively below, so only take a plain path at least two levels deep
# (never / or /var), outside the system's own trees.
if [[ -z $PHOTOS_DIR && -f $PHOTOS_DROPIN ]]; then
  PHOTOS_DIR=$(sed -n 's/^ReadWritePaths=-\{0,1\}//p' "$PHOTOS_DROPIN" | head -n1)
fi
PHOTOS_DIR=${PHOTOS_DIR:-$DEFAULT_PHOTOS_DIR}
# Resolve .. and symlinks, but only for a path that was absolute to begin with.
[[ $PHOTOS_DIR != /* ]] || PHOTOS_DIR=$(realpath -m -- "$PHOTOS_DIR")
if [[ ! $PHOTOS_DIR =~ ^(/[A-Za-z0-9._+-]+){2,}$ ]] ||
   [[ $PHOTOS_DIR =~ ^/(bin|boot|dev|etc|lib|proc|run|sbin|sys|usr)/ ]]; then
  echo "Not a usable photos folder: $PHOTOS_DIR" >&2
  echo "Give a path like /var/pictures or /srv/photos (letters, digits, . _ + -)." >&2
  exit 2
fi
if [[ -e $PHOTOS_DIR && ! -d $PHOTOS_DIR ]]; then
  echo "Photos folder $PHOTOS_DIR exists and is not a folder" >&2
  exit 2
fi

echo "==> Installing binary"
install -m 0755 "$BIN" /usr/local/bin/photoframe
/usr/local/bin/photoframe -version

echo "==> Service user"
if ! id photoframe &>/dev/null; then
  useradd --system --home-dir /var/lib/photoframe --shell /usr/sbin/nologin photoframe
fi
for g in spi gpio; do
  if getent group "$g" >/dev/null; then usermod -aG "$g" photoframe; fi
done

echo "==> Photos folder $PHOTOS_DIR"
mkdir -p "$PHOTOS_DIR"
# -h: a symlink in the folder changes hands itself, not the file it points to.
chown -hR photoframe:photoframe "$PHOTOS_DIR"
# Open to everyone, so any user can copy photos in (scp, rsync, cp) without
# sudo, and replace or remove them later even though each sync makes the
# photos the service's own. Sub-folders too, as they were just handed over.
find "$PHOTOS_DIR" -type d -exec chmod 0777 {} +
# The default was /var/photoframe for a few versions; drop the folder those
# made if nothing was ever put in it.
[[ $PHOTOS_DIR == /var/photoframe ]] || rmdir /var/photoframe 2>/dev/null || true
if [[ $PHOTOS_DIR == "$DEFAULT_PHOTOS_DIR" ]]; then
  rm -f "$PHOTOS_DROPIN"
  rmdir "$(dirname "$PHOTOS_DROPIN")" 2>/dev/null || true
else
  mkdir -p "$(dirname "$PHOTOS_DROPIN")"
  cat > "$PHOTOS_DROPIN" <<DROPIN
# Written by photoframe's install.sh (--photos-dir): the folder for photos
# copied onto the Pi, writable to the service so it can set their mode and
# owner. Removed by quickstart.sh --uninstall.
[Service]
ReadWritePaths=-$PHOTOS_DIR
DROPIN
fi

echo "==> Enabling SPI for the Inky panel"
CONFIG=/boot/firmware/config.txt
[[ -f $CONFIG ]] || CONFIG=/boot/config.txt
REBOOT=0
missing=()
grep -qE '^\s*dtparam=spi=on' "$CONFIG" || missing+=("dtparam=spi=on")
# The driver toggles chip-select itself on GPIO 8, so the kernel must not claim it.
grep -qE '^\s*dtoverlay=spi0-0cs' "$CONFIG" || missing+=("dtoverlay=spi0-0cs")
if (( ${#missing[@]} )); then
  cp "$CONFIG" "$CONFIG.photoframe.bak"
  {
    echo ""
    echo "[all]"
    echo "# Added by photoframe install.sh"
    printf '%s\n' "${missing[@]}"
  } >> "$CONFIG"
  echo "    added ${missing[*]} to $CONFIG (backup: $CONFIG.photoframe.bak)"
  REBOOT=1
fi

if [[ -n $NEW_HOSTNAME ]] && [[ $(hostname) != "$NEW_HOSTNAME" ]]; then
  echo "==> Setting hostname to $NEW_HOSTNAME (app at http://$NEW_HOSTNAME.local)"
  OLD=$(hostname)
  hostnamectl set-hostname "$NEW_HOSTNAME"
  sed -i "s/\b$OLD\b/$NEW_HOSTNAME/g" /etc/hosts
  systemctl restart avahi-daemon 2>/dev/null || true
fi

# Announce the app on the network over mDNS, so the Android app's "Find on
# this network" lists the frame and can reach photoframe.local on phones
# whose resolver can't. Avahi (part of Raspberry Pi OS, and what already
# answers for <hostname>.local) does the announcing from this file: nothing
# runs in photoframe for it. The port is the one the unit listens on.
AVAHI_SERVICES=/etc/avahi/services
if [[ -d $AVAHI_SERVICES ]]; then
  PORT=$(sed -n 's/.*-listen [^ ]*:\([0-9][0-9]*\).*/\1/p' "$HERE/photoframe.service" | head -n1)
  echo "==> Announcing the app over mDNS (_photoframe._tcp, port ${PORT:-80})"
  cat > "$AVAHI_SERVICES/photoframe.service" <<AVAHI
<?xml version="1.0" standalone='no'?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<!-- Written by photoframe's install.sh: the frame's app, for the Android
     app's "Find on this network". Removed by quickstart.sh --uninstall. -->
<service-group>
  <name replace-wildcards="yes">%h</name>
  <service>
    <type>_photoframe._tcp</type>
    <port>${PORT:-80}</port>
    <txt-record>path=/</txt-record>
  </service>
</service-group>
AVAHI
  # Avahi picks up new service files by itself; a reload makes it immediate.
  systemctl reload avahi-daemon 2>/dev/null || true
fi

echo "==> Installing systemd service"
install -m 0644 "$HERE/photoframe.service" /etc/systemd/system/photoframe.service
systemctl daemon-reload
systemctl enable photoframe.service >/dev/null

# Start it either way: the app runs without a panel (or before SPI is up)
# and starts drawing once the panel is there.
systemctl restart photoframe.service
echo
echo "Photoframe is running. Open http://$(hostname).local on your phone."
echo "Logs: journalctl -u photoframe -f"
if (( REBOOT )); then
  echo "SPI was just enabled and turns on after a reboot: sudo reboot"
fi
