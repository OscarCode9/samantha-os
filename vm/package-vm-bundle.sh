#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$ROOT_DIR/vm/dist"
ARCHIVE_PATH="$DIST_DIR/elementary-claw-vm-share.tar.gz"
BUILD_DIR="$ROOT_DIR/references/initial-setup/build"
BUILD_ARCHIVE_PATH="$BUILD_DIR/elementary-claw-vm-share.tar.gz"

export COPYFILE_DISABLE=1

mkdir -p "$DIST_DIR"
mkdir -p "$BUILD_DIR"
rm -f "$ARCHIVE_PATH"
rm -f "$BUILD_ARCHIVE_PATH"

tar -czf "$ARCHIVE_PATH" \
  --no-mac-metadata \
  --exclude='references/initial-setup/.git' \
  --exclude='references/initial-setup/.github' \
  --exclude='references/initial-setup/build' \
  --exclude='._*' \
  --exclude='vm/dist' \
  -s ',^,elementary-claw-vm/,' \
  -C "$ROOT_DIR" \
  go.mod \
  initial-setup-vm-runbook.md \
  cmd \
  deployments \
  internal \
  vm \
  references/initial-setup

cp "$ARCHIVE_PATH" "$BUILD_ARCHIVE_PATH"

echo "Created VM bundle: $ARCHIVE_PATH"
echo "Copied VM bundle to: $BUILD_ARCHIVE_PATH"