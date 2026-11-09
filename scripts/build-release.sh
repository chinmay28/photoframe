#!/usr/bin/env bash
# Build the release bundles for the Raspberry Pi.
#
#   scripts/build-release.sh
#
# Writes to dist/:
#   photoframe-<version>-linux-arm64.tar.gz   64-bit Raspberry Pi OS (Pi Zero 2 W, 3, 4, 5)
#   photoframe-<version>-linux-armv7.tar.gz   32-bit Raspberry Pi OS on an ARMv7 Pi
#   SHA256SUMS
#
# Each tarball holds one directory, photoframe-<version>-linux-<arch>/, with
# the binary as `photoframe` next to install.sh and photoframe.service, so
#
#   tar xzf photoframe-*.tar.gz && cd photoframe-*/ && sudo ./install.sh
#
# installs it. scripts/quickstart.sh does exactly that.
#
# The version comes from scripts/version.sh: vYEAR.MONTH.PATCH, with PATCH as
# the commit count. A shallow clone has no real count and builds patch 0; the
# release workflow refuses to publish that.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="$(scripts/version.sh)"
LDFLAGS="-s -w $(scripts/version.sh --ldflags)"
DIST=dist

if [[ "$(scripts/version.sh --patch)" == 0 ]]; then
  echo "warn: patch is 0, so this is an unstamped build, not a release." >&2
fi

echo "==> photoframe release build ${VERSION}"
rm -rf "${DIST}"
mkdir -p "${DIST}"

bundle() {
  local arch="$1" goarch="$2" goarm="${3:-}"
  local name="photoframe-${VERSION}-linux-${arch}"
  local dir="${DIST}/${name}"

  echo "    ${name}"
  mkdir -p "${dir}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" GOARM="${goarm}" \
    go build -trimpath -ldflags="${LDFLAGS}" -o "${dir}/photoframe" ./cmd/photoframe
  install -m 0755 deploy/install.sh "${dir}/install.sh"
  install -m 0644 deploy/photoframe.service "${dir}/photoframe.service"
  tar -C "${DIST}" -czf "${dir}.tar.gz" "${name}"
  rm -rf "${dir}"
}

bundle arm64 arm64
bundle armv7 arm 7

(cd "${DIST}" && sha256sum photoframe-*.tar.gz > SHA256SUMS)
echo "==> done"
ls -l "${DIST}"
