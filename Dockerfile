# syntax=docker/dockerfile:1
FROM golang:1.26.5-alpine AS builder
ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates

# github.com/hanzoai/* is PRIVATE and must be fetched direct. Without GOPRIVATE
# Go asks the public proxy, which for commerce@v1.36.5 still serves the bits from
# before that repo went private -- a different zip than the tag points at now --
# so `go mod download` died against a go.sum that records the direct hash:
#
#   verifying github.com/hanzoai/commerce@v1.36.5: checksum mismatch
#   SECURITY ERROR
#
# go.sum is CORRECT and must not be "fixed" to the proxy value; verified by
# measuring both ways. luxfi/* is deliberately absent here -- it is public, and
# resolving it through the proxy is what pins it to bytes the sumdb signed.
ENV GOPRIVATE=github.com/hanzoai/*

WORKDIR /app
COPY go.mod go.sum ./

# The credential arrives as a mounted secret, not ARG GITHUB_TOKEN. Two reasons
# the old form could not work: the workflow passed neither build-args nor
# secrets, so the value was empty and the rewrite expanded to a useless
# `https://@github.com/`; and a build ARG is recorded in image history, so a real
# token would have shipped inside the published image. The mount exists only for
# the lifetime of this RUN.
RUN --mount=type=secret,id=gh_token \
    if [ -s /run/secrets/gh_token ]; then \
      git config --global url."https://x-access-token:$(cat /run/secrets/gh_token)@github.com/".insteadOf "https://github.com/"; \
    else \
      echo "gh_token secret is empty; private github.com/hanzoai/* modules will not resolve" >&2; \
    fi; \
    go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o brokerd ./cmd/brokerd/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/brokerd /usr/local/bin/brokerd
EXPOSE 8090 9090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/healthz || exit 1

ENTRYPOINT ["brokerd"]
