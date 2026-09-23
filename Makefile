BINARY := server-health

.PHONY: dev-api dev-web build test install clean fmt

## Run the Go API server (frontend served from web/dist if built).
dev-api:
	go run ./cmd/server-health -addr 127.0.0.1:8080 -services server-health

## Run the Vite dev server with /api proxied to the Go server on :8080.
dev-web:
	cd web && npm run dev

## Build the embedded frontend, then the single Go binary.
build:
	cd web && npm ci --no-audit --no-fund && npm run build
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/server-health

## Vet and test the Go code.
test:
	go vet ./...
	go test ./...

## Install on a Ubuntu server (run as root, or with sudo).
install: clean
	sudo ./deploy/install.sh

clean:
	rm -f $(BINARY)

fmt:
	cd web && npx tsc --noEmit
	gofmt -w ./cmd ./internal ./web/embed.go
