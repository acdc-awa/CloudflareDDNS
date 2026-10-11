.PHONY: build build-linux test vet clean

VERSION := 0.1.0
LDFLAGS := -s -w

build:
	go build -o ddns ./cmd/ddns

build-linux:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/ddns-linux-amd64 ./cmd/ddns
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o dist/ddns-linux-arm64 ./cmd/ddns

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f ddns ddns.exe
	rm -rf dist .gocache
