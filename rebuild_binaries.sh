#!/bin/bash
# Rebuild all GOLD-MD prebuilt binaries with FIXED automsg code
# (same settings as original binaries — verified via `go version -m`)
set -e
export PATH=/usr/local/go/bin:$PATH
cd /workspace/TEST-BOT

echo "── [1/5] gold-md-cgofree  (linux/amd64, CGO_ENABLED=0, -s -w)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o gold-md-cgofree ./src
ls -lh gold-md-cgofree

echo "── [2/5] gold-md-lowram    (linux/amd64, CGO_ENABLED=0, -trimpath)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -o gold-md-lowram ./src
ls -lh gold-md-lowram

echo "── [3/5] gold-md-bot-static(linux/amd64, CGO_ENABLED=0, -s -w)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o gold-md-bot-static ./src
ls -lh gold-md-bot-static

echo "── [4/5] gold-md-bot-arm64 (linux/arm64, CGO_ENABLED=0, -s -w)"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o gold-md-bot-arm64 ./src
ls -lh gold-md-bot-arm64

echo "── [5/5] gold-md-freebsd   (freebsd/amd64, CGO_ENABLED=0, -s -w)"
CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o gold-md-freebsd ./src
ls -lh gold-md-freebsd

echo ""
echo "ALL_BINARIES_REBUILT_OK"
