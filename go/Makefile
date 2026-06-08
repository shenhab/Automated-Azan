# Automated Azan Agent — Go Build
BINARY  := azan-agent
MODULE  := azan-agent
CMD     := ./cmd/azan-agent

# ── local build ──────────────────────────────────────────────────────────────
.PHONY: build run tidy install-service

build:
	go build -o $(BINARY) $(CMD)

run:
	go run $(CMD)

tidy:
	go mod tidy

# ── cross-compile (all platforms from Linux/macOS with Go installed) ──────────
.PHONY: build-all build-windows build-mac-intel build-mac-arm build-linux build-pi

build-all: build-windows build-mac-intel build-mac-arm build-linux build-pi

build-windows:
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe $(CMD)

build-mac-intel:
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64      $(CMD)

build-mac-arm:
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64      $(CMD)

build-linux:
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64       $(CMD)

build-pi:
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64       $(CMD)

# ── service management (run on target machine after build) ────────────────────
install-service:
	./$(BINARY) install

uninstall-service:
	./$(BINARY) uninstall

# ── dev helpers ───────────────────────────────────────────────────────────────
.PHONY: vet lint clean

vet:
	go vet ./...

clean:
	rm -f $(BINARY) dist/*
