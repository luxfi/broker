.PHONY: build run test test-race lint proto docker docker-push tag clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o brokerd ./cmd/brokerd/

run: build
	ALPACA_API_KEY=$(ALPACA_API_KEY) ALPACA_API_SECRET=$(ALPACA_API_SECRET) ./brokerd

test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

lint:
	go vet ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/broker.proto

docker:
	docker build --platform linux/amd64 -t ghcr.io/luxfi/broker:$(VERSION) -t ghcr.io/luxfi/broker:latest .

docker-push:
	docker push ghcr.io/luxfi/broker:$(VERSION)
	docker push ghcr.io/luxfi/broker:latest

tag:
	@echo "Current version: $(VERSION)"
	@echo "Usage: git tag -a v0.X.0 -m 'release v0.X.0' && git push origin v0.X.0"

clean:
	rm -f brokerd
