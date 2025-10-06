#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BUILD_DIR="$ROOT_DIR/build"
DIST_DIR="$ROOT_DIR/dist"
BINARY_NAME="rueDeLegliseBooker"
ARCHIVE_NAME="${BINARY_NAME}.tgz"

rm -rf "$BUILD_DIR" "$DIST_DIR"
mkdir -p "$BUILD_DIR" "$DIST_DIR"

GOOS="${GOOS:-}" GOARCH="${GOARCH:-}" CGO_ENABLED="${CGO_ENABLED:-1}" go build -o "$BUILD_DIR/$BINARY_NAME" .

cp "$ROOT_DIR/config.json" "$BUILD_DIR/"
cp -R "$ROOT_DIR/static" "$BUILD_DIR/"
cp -R "$ROOT_DIR/templates" "$BUILD_DIR/"

( cd "$BUILD_DIR" && tar -czf "$DIST_DIR/$ARCHIVE_NAME" "$BINARY_NAME" config.json static templates )

echo "Archive created at $DIST_DIR/$ARCHIVE_NAME"
