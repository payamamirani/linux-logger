GO      := go
DAEMON  := payam-loggerd

.PHONY: build run fmt vet test clean install-user install-extension

## build: compile the daemon into ./bin
build:
	$(GO) build -o bin/$(DAEMON) ./cmd/$(DAEMON)

## run: build then run the daemon in the foreground
run: build
	./bin/$(DAEMON)

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
install-user: build
	./scripts/install.sh

## install-extension: install the GNOME Shell extension (silent capture)
install-extension:
	./scripts/install-extension.sh

## clean: remove build artifacts
clean:
	rm -rf bin
