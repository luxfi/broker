.PHONY: build run test docker

build:
	go build -o brokerd ./cmd/brokerd/

run: build
	ALPACA_API_KEY=$(ALPACA_API_KEY) ALPACA_API_SECRET=$(ALPACA_API_SECRET) ./brokerd

test:
	go test ./...

docker:
	docker build --platform linux/amd64 -t ghcr.io/luxfi/broker:latest .

clean:
	rm -f brokerd
