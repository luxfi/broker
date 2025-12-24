# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-certificates

ARG GITHUB_TOKEN
RUN git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
ENV GOPRIVATE=github.com/luxfi/*,github.com/hanzoai/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o brokerd ./cmd/brokerd/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/brokerd /usr/local/bin/brokerd
EXPOSE 8090 9090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/healthz || exit 1

ENTRYPOINT ["brokerd"]
