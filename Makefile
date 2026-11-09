VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GO      ?= go
PI      ?= pi@photoframe.local

.PHONY: all build pi pi32 test vet dev deploy clean

all: vet test pi

build:
	$(GO) build -ldflags '$(LDFLAGS)' -o dist/photoframe ./cmd/photoframe

# Raspberry Pi Zero 2 W running 64-bit Raspberry Pi OS (recommended).
pi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags '$(LDFLAGS)' -o dist/photoframe-linux-arm64 ./cmd/photoframe

# 32-bit Raspberry Pi OS.
pi32:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -ldflags '$(LDFLAGS)' -o dist/photoframe-linux-armv7 ./cmd/photoframe

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt" && exit 1)

# Run on a laptop: frames are written to devdata/display.png.
dev:
	$(GO) run ./cmd/photoframe -data ./devdata -display png -listen :8080

# Copy to the Pi and install (override PI=user@host).
deploy: pi
	ssh $(PI) 'mkdir -p /tmp/photoframe-install'
	scp dist/photoframe-linux-arm64 deploy/photoframe.service deploy/install.sh $(PI):/tmp/photoframe-install/
	ssh -t $(PI) 'cd /tmp/photoframe-install && sudo ./install.sh --binary ./photoframe-linux-arm64'

clean:
	rm -rf dist devdata
