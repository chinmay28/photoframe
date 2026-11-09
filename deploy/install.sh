#!/usr/bin/env bash
# Install or upgrade photoframe on a Raspberry Pi (Raspberry Pi OS Bookworm
# or later). Run on the Pi, as root, from a directory containing the binary
# and photoframe.service:
#
#   sudo ./install.sh [--binary ./photoframe-linux-arm64] [--hostname photoframe]
#
# Safe to re-run: it only changes what is missing.
set -euo pipefail

BIN="./photoframe"
NEW_HOSTNAME=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BIN="$2"; shift 2 ;;
    --hostname) NEW_HOSTNAME="$2"; shift 2 ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

HERE="$(cd "$(dirname "$0")" && pwd)"
[[ $(id -u) -eq 0 ]] || { echo "Run as root: sudo $0 $*" >&2; exit 1; }
[[ -x "$BIN" ]] || { echo "Binary not found or not executable: $BIN" >&2; exit 1; }

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

echo "==> Installing systemd service"
install -m 0644 "$HERE/photoframe.service" /etc/systemd/system/photoframe.service
systemctl daemon-reload
systemctl enable photoframe.service >/dev/null

if (( REBOOT )); then
  echo
  echo "SPI was just enabled. Reboot to start the frame:  sudo reboot"
else
  systemctl restart photoframe.service
  echo
  echo "Photoframe is running. Open http://$(hostname).local on your phone."
  echo "Logs: journalctl -u photoframe -f"
fi
