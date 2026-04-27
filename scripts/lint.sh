#!/usr/bin/env sh
set -eu

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint is required. Install it from https://golangci-lint.run/welcome/install/" >&2
  exit 1
fi

golangci-lint run ./...
