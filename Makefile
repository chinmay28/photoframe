# The version is vYEAR.MONTH.PATCH: the commit's month and the commit count,
# stamped in at link time (see internal/version and scripts/version.sh). A
# bare `go build` leaves the patch at 0, which marks a development build.
LDFLAGS := -s -w $(shell scripts/version.sh --ldflags 2>/dev/null)
GO      ?= go
PI      ?= pi@photoframe.local

.PHONY: all build pi pi32 test vet dev deploy release version clean

all: vet test pi

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/photoframe ./cmd/photoframe

# Print the version this tree builds as.
version:
	@scripts/version.sh

# Release tarballs for the Pi (arm64 and armv7) plus SHA256SUMS, in dist/.
release:
	scripts/build-release.sh

# Raspberry Pi Zero 2 W running 64-bit Raspberry Pi OS (recommended).
pi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/photoframe-linux-arm64 ./cmd/photoframe

# 32-bit Raspberry Pi OS.
pi32:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/photoframe-linux-armv7 ./cmd/photoframe

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt" && exit 1)

# Run on a laptop: frames are written to devdata/display.png.
dev:
	$(GO) run -ldflags '$(LDFLAGS)' ./cmd/photoframe -data ./devdata -display png -listen :8080

# Copy to the Pi and install (override PI=user@host).
deploy: pi
	ssh $(PI) 'mkdir -p /tmp/photoframe-install'
	scp dist/photoframe-linux-arm64 deploy/photoframe.service deploy/install.sh $(PI):/tmp/photoframe-install/
	ssh -t $(PI) 'cd /tmp/photoframe-install && sudo ./install.sh --binary ./photoframe-linux-arm64'

clean:
	rm -rf dist devdata
