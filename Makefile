GO      := go
BINARY  := payam-loggerd

.PHONY: build run fmt vet test clean install-user

## build: compile the daemon to backend/bin/payam-loggerd
build:
	$(GO) build -C backend -o bin/$(BINARY) ./cmd/payam-loggerd

## run: build then run the daemon in the foreground
run: build
	./backend/bin/$(BINARY)

## fmt: gofmt all packages
fmt:
	$(GO) fmt -C backend ./...

## vet: run go vet
vet:
	$(GO) vet -C backend ./...

## test: run unit tests
test:
	$(GO) test -C backend ./...

## install-user: install binary + systemd user service and enable it
install-user: build
	./scripts/install.sh

## clean: remove build artifacts
clean:
	rm -rf backend/bin
