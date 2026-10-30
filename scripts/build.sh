#!/usr/bin/env bash
# 交叉编译 crazynote：飞牛 NAS 有 x86 与 ARM 两种常见架构
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="dist"
VERSION="${VERSION:-1.0.0}"
LDFLAGS="-s -w -X main.version=${VERSION}"

mkdir -p "$OUT"

echo "==> 编译 linux/amd64 (x86_64)"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" \
  -o "$OUT/crazynote-linux-amd64" ./cmd/crazynote

echo "==> 编译 linux/arm64 (aarch64)"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" \
  -o "$OUT/crazynote-linux-arm64" ./cmd/crazynote

echo "==> 编译 linux/arm (armv7l)"
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" \
  -o "$OUT/crazynote-linux-armv7" ./cmd/crazynote

echo "==> 编译 windows/amd64 (本地调试用)"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" \
  -o "$OUT/CrazyNote.exe" ./cmd/crazynote

echo "==> 编译 darwin/arm64 (Mac 调试用)"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$LDFLAGS" \
  -o "$OUT/crazynote-darwin-arm64" ./cmd/crazynote

echo ""
echo "产物列表："
ls -lh "$OUT"
