GO      := go
DAEMON  := payam-loggerd
UI      := payam-logger-ui

.PHONY: build build-daemon build-ui run run-ui fmt vet test clean install-user install-extension

## build: compile daemon + UI into ./bin
build: build-daemon build-ui

## build-daemon: compile the background service
build-daemon:
	$(GO) build -o bin/$(DAEMON) ./cmd/$(DAEMON)

## build-ui: compile the GTK4 viewer (needs libgtk-4-dev + pkgconf)
build-ui:
	$(GO) build -o bin/$(UI) ./cmd/$(UI)

## run: build then run the daemon in the foreground
run: build-daemon
	./bin/$(DAEMON)

## run-ui: build then run the GTK4 viewer
run-ui: build-ui
	./bin/$(UI)

## fmt: gofmt all packages
fmt:
	$(GO) fmt ./...

## vet: run go vet
vet:
	$(GO) vet ./...

## test: run unit tests
test:
	$(GO) test ./...

## install-user: install binary + systemd user service and enable it
install-user: build-daemon
	./scripts/install.sh

## install-extension: install the GNOME Shell extension (silent capture)
install-extension:
	./scripts/install-extension.sh

## clean: remove build artifacts
clean:
	rm -rf bin
