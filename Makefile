.PHONY: build build-linux release lint vuln check

VERSION ?= dev
LDFLAGS := -s -w
BIN := .

build:
	go build -ldflags="$(LDFLAGS)" -o bin/openbao-plugin-secrets-garage $(BIN)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/openbao-plugin-secrets-garage-linux-amd64 $(BIN)

release:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/openbao-plugin-secrets-garage_$(VERSION)_linux_amd64 $(BIN)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/openbao-plugin-secrets-garage_$(VERSION)_linux_arm64 $(BIN)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/openbao-plugin-secrets-garage_$(VERSION)_darwin_amd64 $(BIN)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/openbao-plugin-secrets-garage_$(VERSION)_darwin_arm64 $(BIN)
	cd dist && sha256sum openbao-plugin-secrets-garage_* > SHA256SUMS

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

test:
	go test ./...

check: lint vuln test