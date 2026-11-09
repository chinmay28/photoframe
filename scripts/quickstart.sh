#!/usr/bin/env bash
#
# Photoframe: one-command install for a Raspberry Pi (Raspberry Pi OS Bookworm
# or later). Run it on the Pi:
#
#   curl -fsSL https://raw.githubusercontent.com/chinmay28/photoframe/main/scripts/quickstart.sh | sudo bash
#
# Run the same command again to upgrade. Add a flag to remove it, keeping your
# photos and settings:
#
#   curl -fsSL …/quickstart.sh | sudo bash -s -- --uninstall
#
# By default it installs what is on main right now. CI builds every push to
# main and publishes it as the "edge" pre-release; that build is used when it
# is of main's newest commit, otherwise main is built here from source.
# PHOTOFRAME_RELEASE=latest installs the newest tagged release instead.
#
# It gets a binary one of two ways (PHOTOFRAME_INSTALL picks):
#
#   release  download the prebuilt bundle for this Pi from a GitHub release
#            (PHOTOFRAME_RELEASE) and check it against the release's
#            SHA256SUMS. Takes seconds and needs no toolchain.
#   source   clone the repository and build it here, installing Go if it is
#            missing or too old. Works on any ARM Pi and can build any
#            branch, tag or commit, but takes several minutes on a Pi Zero 2 W.
#   auto     (default) release if a suitable one exists for this Pi, else source.
#
# Running this file from a checkout (sudo ./scripts/quickstart.sh) builds that
# checkout as it stands, committed or not, and never pulls. That is how you
# deploy work in progress.
#
# Either way the bundle is installed by deploy/install.sh, the same script
# `make deploy` uses: binary in /usr/local/bin, a `photoframe` system user,
# SPI enabled in config.txt, and photoframe.service enabled.
#
# Safe to re-run:
#
#   * Photos, caches and settings live in /var/lib/photoframe, which this
#     script never deletes (except with --uninstall and PHOTOFRAME_PURGE=1).
#   * Release mode skips the install when that version is already running.
#   * Before an upgrade, the running binary and settings.json are kept. After
#     the restart it waits for the app to answer with the new version. If it
#     doesn't, the old binary and settings go back and the old version is
#     restarted, so a bad upgrade leaves the frame as it was.
#
# Settings (environment variables, all optional):
#
#   PHOTOFRAME_INSTALL   auto | release | source      (default: auto)
#   PHOTOFRAME_RELEASE   edge | latest | <tag>        edge = build of main (default)
#   PHOTOFRAME_BUNDLE    path to a release .tar.gz already on the Pi; installs
#                        it without going online
#   PHOTOFRAME_GITHUB    owner/name on GitHub         (default: chinmay28/photoframe)
#   PHOTOFRAME_REF       branch, tag or commit to build in source mode (default: main)
#   PHOTOFRAME_HOSTNAME  set the Pi's hostname, so the app is at http://<name>.local
#   PHOTOFRAME_FORCE     1: reinstall even if this version is already running
#   PHOTOFRAME_PURGE     1: with --uninstall, also delete /var/lib/photoframe
#                        (uploaded photos and settings) and the service user
#   INSTALL_GO           auto | never                 (default: auto; source mode only)

set -euo pipefail

# This usually runs over ssh. A dropped session must not kill the install
# between swapping the binary and checking the new one, so ignore SIGHUP and
# keep a log to read afterwards.
trap '' HUP

LOG=/var/log/photoframe-quickstart.log

if [ -t 1 ]; then
  C_BLUE=$'\033[1;34m'; C_GREEN=$'\033[1;32m'; C_YELLOW=$'\033[1;33m'; C_RED=$'\033[1;31m'; C_OFF=$'\033[0m'
else
  C_BLUE=''; C_GREEN=''; C_YELLOW=''; C_RED=''; C_OFF=''
fi
say()  { printf '%s\n' "$*" >>"$LOG" 2>/dev/null || :; }
log()  { printf '%s==>%s %s\n' "$C_BLUE" "$C_OFF" "$*" 2>/dev/null || :; say "==> $*"; }
ok()   { printf '%s ok %s %s\n' "$C_GREEN" "$C_OFF" "$*" 2>/dev/null || :; say " ok $*"; }
warn() { printf '%swarn%s %s\n' "$C_YELLOW" "$C_OFF" "$*" >&2 2>/dev/null || :; say "warn $*"; }
die()  { printf '%serr %s %s\n' "$C_RED" "$C_OFF" "$*" >&2 2>/dev/null || :; say "err $*"; exit 1; }

usage() { sed -n '2,/^$/p' "${BASH_SOURCE[0]}" 2>/dev/null | sed 's/^# \{0,1\}//'; }

# ---------------------------------------------------------------------------
# Arguments and preconditions
# ---------------------------------------------------------------------------
UNINSTALL=0
case "${1:-}" in
  "") ;;
  --uninstall) UNINSTALL=1 ;;
  -h|--help) usage; exit 0 ;;
  *) die "unknown option: $1 (use --uninstall, or --help)" ;;
esac

[ "$(id -u)" -eq 0 ] || die "run as root: curl -fsSL …/quickstart.sh | sudo bash"
command -v systemctl >/dev/null 2>&1 || die "systemd is required (no systemctl found)"
say "--- $(date -Is) quickstart ${*:-install}"

INSTALL_MODE="${PHOTOFRAME_INSTALL:-auto}"
RELEASE="${PHOTOFRAME_RELEASE:-edge}"
GITHUB="${PHOTOFRAME_GITHUB:-chinmay28/photoframe}"
REF="${PHOTOFRAME_REF:-main}"
INSTALL_GO="${INSTALL_GO:-auto}"

BIN=/usr/local/bin/photoframe
UNIT=/etc/systemd/system/photoframe.service
DATA_DIR=/var/lib/photoframe
STATE_DIR=/usr/local/lib/photoframe   # the previous binary, for rollback
BACKUP_DIR="$DATA_DIR/backups"
BACKUP_KEEP=10

case "$INSTALL_MODE" in auto|release|source) ;; *) die "PHOTOFRAME_INSTALL must be auto, release or source, not '$INSTALL_MODE'" ;; esac
case "$INSTALL_GO" in auto|never) ;; *) die "INSTALL_GO must be auto or never" ;; esac

# ---------------------------------------------------------------------------
# Uninstall
# ---------------------------------------------------------------------------
if [ "$UNINSTALL" = 1 ]; then
  log "Removing photoframe"
  systemctl disable --now photoframe.service 2>/dev/null || :
  rm -f "$UNIT" "$BIN"
  rm -rf "$STATE_DIR" /var/cache/photoframe-build
  systemctl daemon-reload
  if [ "${PHOTOFRAME_PURGE:-0}" = 1 ]; then
    rm -rf "$DATA_DIR"
    userdel photoframe 2>/dev/null || :
    ok "removed, including $DATA_DIR and the photoframe user"
  else
    ok "removed; photos and settings kept in $DATA_DIR (PHOTOFRAME_PURGE=1 deletes them)"
  fi
  echo "SPI stays enabled in config.txt. The original is next to it as config.txt.photoframe.bak, if the installer changed it."
  exit 0
fi

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
# /var/tmp, not /tmp: on recent Raspberry Pi OS /tmp is a RAM disk, and a
# 512 MB Pi Zero has no RAM to spare for a Go build.
WORK="$(mktemp -d /var/tmp/photoframe-quickstart.XXXXXX)"
# Go's build and module caches survive between runs, so a build that is cut
# off (a dropped ssh session, a deploy tool's timeout) resumes where it stopped
# when the script is run again instead of starting over.
BUILD_CACHE=/var/cache/photoframe-build
SWAPFILE="$BUILD_CACHE/swapfile"
SWAP_ADDED=0
cleanup() {
  if [ "$SWAP_ADDED" = 1 ]; then
    swapoff "$SWAPFILE" 2>/dev/null || :
    rm -f "$SWAPFILE"
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

ensure_pkg() { # ensure_pkg <command> <apt package>
  command -v "$1" >/dev/null 2>&1 && return
  command -v apt-get >/dev/null 2>&1 || die "$1 is missing and there is no apt-get to install it"
  log "Installing $2"
  if [ -z "${APT_UPDATED:-}" ]; then apt-get update -qq; APT_UPDATED=1; fi
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$2" >/dev/null
}

# The release arch for this Pi, or empty when no release is built for it.
release_arch() {
  case "$(uname -m)" in
    aarch64|arm64) echo arm64 ;;
    armv7l|armv8l) echo armv7 ;;
    *) echo "" ;;
  esac
}

installed_version() { [ -x "$BIN" ] && "$BIN" -version 2>/dev/null | awk '{print $NF}' || :; }

# git in a tree root may not own (a checkout made by the pi user): allow it,
# or version.sh quietly gets no commit count and builds patch 0.
git_any() { GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*' "$@"; }

# ---------------------------------------------------------------------------
# Getting a bundle: a directory with photoframe, install.sh, photoframe.service
# ---------------------------------------------------------------------------
BUNDLE=""
NEW_VERSION=""

unpack() { # unpack <tarball>
  tar -C "$WORK" -xzf "$1"
  BUNDLE="$(find "$WORK" -mindepth 2 -maxdepth 2 -name photoframe -type f -printf '%h\n' | head -n1)"
  [ -n "$BUNDLE" ] && [ -f "$BUNDLE/install.sh" ] && [ -f "$BUNDLE/photoframe.service" ] \
    || die "$1 is not a photoframe release bundle"
  chmod +x "$BUNDLE/photoframe" "$BUNDLE/install.sh"
  NEW_VERSION="$("$BUNDLE/photoframe" -version | awk '{print $NF}')" \
    || die "the binary in $1 doesn't run on this Pi ($(uname -m))"
}

# github_api <path>: GET https://api.github.com/repos/$GITHUB/<path>, or
# nothing when it fails (offline, rate limited, not found).
github_api() {
  curl -fsSL -H 'Accept: application/vnd.github+json' "https://api.github.com/repos/$GITHUB/$1" 2>/dev/null || :
}

# json_field <name>: the first string value of "name" in JSON on stdin. The
# API pretty-prints, one field per line, which is all this needs.
json_field() {
  sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n1
}

# from_release succeeds with a bundle, or fails (return 1) when there is no
# suitable release, so auto mode can fall back to a source build.
from_release() {
  local arch tag name base version
  arch="$(release_arch)"
  if [ -z "$arch" ]; then
    warn "no release is built for $(uname -m)"
    return 1
  fi
  ensure_pkg curl curl
  tag="$RELEASE"
  case "$tag" in
    latest)
      tag="$(github_api releases/latest | json_field tag_name)"
      if [ -z "$tag" ]; then
        warn "no release found for $GITHUB"
        return 1
      fi ;;
    edge)
      # CI publishes every push to main as the "edge" pre-release. Use it only
      # when it is main's newest commit, so "install main" means just that.
      if [ "$REF" != main ]; then
        log "edge builds only main; building $REF from source"
        return 1
      fi
      local built head
      built="$(github_api releases/tags/edge | json_field target_commitish)"
      if [ -z "$built" ]; then
        warn "no build of main has been published yet"
        return 1
      fi
      head="$(curl -fsSL -H 'Accept: application/vnd.github.sha' "https://api.github.com/repos/$GITHUB/commits/main" 2>/dev/null || :)"
      if [ -z "$head" ]; then
        warn "could not ask GitHub for main's newest commit; installing the published build of ${built:0:7}"
      elif [ "$built" != "$head" ]; then
        warn "the published build is of ${built:0:7}, but main is at ${head:0:7} (CI may still be building it)"
        return 1
      fi ;;
  esac

  # The bundle's name, and so its version, comes from the release's SHA256SUMS:
  # edge's file names change with every commit.
  base="https://github.com/$GITHUB/releases/download/$tag"
  curl -fsSL -o "$WORK/SHA256SUMS" "$base/SHA256SUMS" || { warn "release $tag has no SHA256SUMS"; return 1; }
  name="$(awk -v want="-linux-$arch.tar.gz" 'substr($2, length($2) - length(want) + 1) == want {print $2; exit}' "$WORK/SHA256SUMS")"
  if [ -z "$name" ]; then
    warn "release $tag has no bundle for linux-$arch"
    return 1
  fi
  version="${name#photoframe-}"
  version="${version%-linux-$arch.tar.gz}"
  if [ "$version" = "$(installed_version)" ] && [ "${PHOTOFRAME_FORCE:-0}" != 1 ]; then
    ok "photoframe $version is already installed (PHOTOFRAME_FORCE=1 reinstalls it)"
    exit 0
  fi

  log "Downloading $name ($tag)"
  curl -fsSL -o "$WORK/$name" "$base/$name" || { warn "could not download $base/$name"; return 1; }
  (cd "$WORK" && grep " $name\$" SHA256SUMS | sha256sum -c --status) \
    || die "$name does not match the release's SHA256SUMS"
  ok "checksum matches"
  unpack "$WORK/$name"
}

# The checkout this file is in, if it was run from one rather than piped.
local_checkout() {
  local src="${BASH_SOURCE[0]:-}" dir
  [ -f "$src" ] || return 1
  dir="$(cd "$(dirname "$src")/.." && pwd)"
  [ -f "$dir/go.mod" ] && [ -d "$dir/cmd/photoframe" ] && grep -q 'module github.com/chinmay28/photoframe' "$dir/go.mod" \
    && echo "$dir"
}

# go_ok succeeds when the go on PATH is at least the version go.mod asks for.
go_ok() { # go_ok <required, e.g. 1.24>
  command -v go >/dev/null 2>&1 || return 1
  local have
  have="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  [ "$(printf '%s\n%s\n' "$1" "$have" | sort -V | head -n1)" = "$1" ]
}

ensure_go() { # ensure_go <required>
  export PATH="/usr/local/go/bin:$PATH"
  go_ok "$1" && return
  [ "$INSTALL_GO" = auto ] || die "Go $1 or later is needed to build (INSTALL_GO=never)"
  local arch latest
  case "$(uname -m)" in
    aarch64|arm64) arch=arm64 ;;
    armv6l|armv7l|armv8l) arch=armv6l ;;
    x86_64) arch=amd64 ;;
    *) die "no Go download for $(uname -m)" ;;
  esac
  ensure_pkg curl curl
  latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)"
  [ -n "$latest" ] || die "could not find the current Go version at go.dev"
  log "Installing $latest to /usr/local/go"
  curl -fsSL "https://go.dev/dl/$latest.linux-$arch.tar.gz" -o "$WORK/go.tgz"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$WORK/go.tgz"
  go_ok "$1" || die "installed $latest, but the build needs Go $1"
}

# ensure_build_memory adds a temporary swap file when RAM plus swap is too
# small to compile: the Go compiler on a 512 MB Pi Zero 2 W with the default
# 100 MB of swap thrashes until the board stops answering, ssh included.
ensure_build_memory() {
  local need_kb=1572864 have_kb add_mb free_mb
  have_kb="$(awk '/^(MemTotal|SwapTotal):/ {t += $2} END {print t}' /proc/meminfo)"
  [ "$have_kb" -ge "$need_kb" ] && return
  add_mb=$(( (need_kb - have_kb) / 1024 + 64 ))
  free_mb="$(df -Pm "$BUILD_CACHE" | awk 'NR == 2 {print $4}')"
  if [ "$free_mb" -lt $((add_mb + 512)) ]; then
    warn "only ${free_mb} MB free on disk: building without extra swap, which may run out of memory"
    return
  fi
  log "Adding ${add_mb} MB of temporary swap for the build (removed afterwards)"
  rm -f "$SWAPFILE"
  if ! { fallocate -l "${add_mb}M" "$SWAPFILE" 2>/dev/null || dd if=/dev/zero of="$SWAPFILE" bs=1M count="$add_mb" status=none; } \
    || ! chmod 600 "$SWAPFILE" || ! mkswap -q "$SWAPFILE" >/dev/null || ! swapon "$SWAPFILE"; then
    rm -f "$SWAPFILE"
    warn "could not add swap; building anyway"
    return
  fi
  SWAP_ADDED=1
}

# build_with_progress runs a command with its output in the log, printing a
# line every 20 s. A quiet multi-minute compile looks hung to people and to
# deploy tools, and writing to a terminal that has gone away (dropped ssh)
# must not kill the build.
build_with_progress() {
  local start pid last
  start=$(date +%s)
  say "\$ $*"
  "$@" >>"$LOG" 2>&1 &
  pid=$!
  local tick=0
  while kill -0 "$pid" 2>/dev/null; do
    sleep 1
    tick=$((tick + 1))
    if [ $((tick % 20)) -eq 0 ] && kill -0 "$pid" 2>/dev/null; then
      last="$(tail -n1 "$LOG" 2>/dev/null | cut -c1-60)"
      printf '    still building (%ss)%s\n' "$(( $(date +%s) - start ))" "${last:+: $last}" 2>/dev/null || :
    fi
  done
  wait "$pid"
}

from_source() {
  local src required goarch goarm
  if src="$(local_checkout)"; then
    log "Building the checkout at $src as it stands (not pulling)"
  else
    ensure_pkg git git
    src="$WORK/src"
    log "Cloning $GITHUB at $REF"
    # --filter=blob:none, not --depth 1: the patch number is the commit count,
    # and a shallow clone has none.
    git clone -q --filter=blob:none "https://github.com/$GITHUB.git" "$src"
    git -C "$src" checkout -q "$REF"
  fi

  required="$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$src/go.mod")"
  ensure_go "${required:-1.24}"

  case "$(uname -m)" in
    aarch64|arm64) goarch=arm64; goarm="" ;;
    armv7l|armv8l) goarch=arm; goarm=7 ;;
    armv6l)        goarch=arm; goarm=6 ;;
    *)             goarch=""; goarm="" ;; # building for this machine
  esac

  local ldflags parallel=()
  ldflags="-s -w $(git_any "$src/scripts/version.sh" --ldflags)"
  # A Pi Zero 2 W has 512 MB. Compiling one package at a time keeps the build
  # from running out of memory, at some cost in time.
  if [ "$(awk '/MemTotal/ {print $2}' /proc/meminfo)" -lt 1048576 ]; then
    parallel=(-p 1)
    log "Building with Go $(go env GOVERSION | sed 's/^go//') (low memory: one package at a time, a few minutes)"
  else
    log "Building with Go $(go env GOVERSION | sed 's/^go//')"
  fi

  mkdir -p "$BUILD_CACHE"
  ensure_build_memory
  BUNDLE="$WORK/bundle"
  mkdir -p "$BUNDLE"
  # Through env: an assignment produced by an expansion is not an assignment.
  # -v prints each package as it compiles, which build_with_progress shows.
  if ! (cd "$src" && build_with_progress env CGO_ENABLED=0 GOOS=linux ${goarch:+GOARCH=$goarch} ${goarm:+GOARM=$goarm} \
    GOCACHE="$BUILD_CACHE/gocache" GOMODCACHE="$BUILD_CACHE/gomodcache" GOPATH="$BUILD_CACHE/gopath" \
    go build -v "${parallel[@]}" -trimpath -ldflags="$ldflags" -o "$BUNDLE/photoframe" ./cmd/photoframe); then
    tail -n 15 "$LOG" >&2 2>/dev/null || :
    die "build failed (full output in $LOG); the installed version, if any, is untouched"
  fi
  install -m 0755 "$src/deploy/install.sh" "$BUNDLE/install.sh"
  install -m 0644 "$src/deploy/photoframe.service" "$BUNDLE/photoframe.service"
  NEW_VERSION="$("$BUNDLE/photoframe" -version | awk '{print $NF}')"
  ok "built photoframe $NEW_VERSION"
}

if [ -n "${PHOTOFRAME_BUNDLE:-}" ]; then
  [ -f "$PHOTOFRAME_BUNDLE" ] || die "PHOTOFRAME_BUNDLE=$PHOTOFRAME_BUNDLE is not a file"
  log "Installing from $PHOTOFRAME_BUNDLE"
  unpack "$PHOTOFRAME_BUNDLE"
else
  case "$INSTALL_MODE" in
    release) from_release || die "no release to install; PHOTOFRAME_INSTALL=source builds from source instead" ;;
    source)  from_source ;;
    auto)
      if local_checkout >/dev/null; then from_source
      elif ! from_release; then log "Falling back to a source build"; from_source
      fi ;;
  esac
fi

# ---------------------------------------------------------------------------
# Install, check, roll back
# ---------------------------------------------------------------------------
OLD_VERSION="$(installed_version)"
SETTINGS_BACKUP=""
mkdir -p "$STATE_DIR"
rm -f "$STATE_DIR/photoframe.prev"
if [ -n "$OLD_VERSION" ]; then
  cp -p "$BIN" "$STATE_DIR/photoframe.prev"
  if [ -f "$DATA_DIR/settings.json" ]; then
    mkdir -p "$BACKUP_DIR"
    SETTINGS_BACKUP="$BACKUP_DIR/settings-$OLD_VERSION-$(date +%Y%m%dT%H%M%S).json"
    cp -p "$DATA_DIR/settings.json" "$SETTINGS_BACKUP"
    # Keep the newest few.
    # Names are ours (settings-<version>-<timestamp>.json), so ls is safe here.
    # shellcheck disable=SC2012
    ls -1t "$BACKUP_DIR"/settings-*.json 2>/dev/null | tail -n +$((BACKUP_KEEP + 1)) | xargs -r rm -f
  fi
  log "Upgrading $OLD_VERSION → $NEW_VERSION"
else
  log "Installing $NEW_VERSION"
fi

if ! (cd "$BUNDLE" && ./install.sh --binary ./photoframe ${PHOTOFRAME_HOSTNAME:+--hostname "$PHOTOFRAME_HOSTNAME"}) 2>&1 | tee -a "$LOG"; then
  die "install.sh failed"
fi

# The port the service listens on, from its unit (-listen :80).
PORT="$(sed -n 's/.*-listen [^ ]*:\([0-9][0-9]*\).*/\1/p' "$UNIT" | head -n1)"
PORT="${PORT:-80}"

# healthy succeeds once the running service answers /api/device with $1.
healthy() {
  local want="$1" body
  for _ in $(seq 1 30); do
    body="$(curl -fsS --max-time 3 "http://127.0.0.1:$PORT/api/device" 2>/dev/null || :)"
    case "$body" in *"\"version\":\"$want\""*) return 0 ;; esac
    sleep 2
  done
  return 1
}

ensure_pkg curl curl
log "Waiting for the app on port $PORT"
if healthy "$NEW_VERSION"; then
  rm -f "$STATE_DIR/photoframe.prev"
  echo
  ok "photoframe $NEW_VERSION is running. Open http://$(hostname).local on your phone."
  echo "    Logs: journalctl -u photoframe -f"
  if [ ! -e /dev/spidev0.0 ]; then
    echo "    SPI is not on yet, so the panel can't be used. Reboot once to turn it on: sudo reboot"
    echo "    (The app works without the panel; attach it with the Pi powered off.)"
  fi
  exit 0
fi

warn "photoframe $NEW_VERSION did not come up. Last log lines:"
journalctl -u photoframe -n 20 --no-pager 2>/dev/null | tee -a "$LOG" >&2 || :
if [ ! -f "$STATE_DIR/photoframe.prev" ]; then
  die "nothing to roll back to on a first install; see journalctl -u photoframe"
fi
log "Rolling back to $OLD_VERSION"
systemctl stop photoframe.service || :
install -m 0755 "$STATE_DIR/photoframe.prev" "$BIN"
if [ -n "$SETTINGS_BACKUP" ]; then
  cp -p "$SETTINGS_BACKUP" "$DATA_DIR/settings.json"
fi
systemctl start photoframe.service
if healthy "$OLD_VERSION"; then
  die "upgrade to $NEW_VERSION failed; $OLD_VERSION is running again with its settings"
fi
die "upgrade to $NEW_VERSION failed and $OLD_VERSION did not come back either; see journalctl -u photoframe"
