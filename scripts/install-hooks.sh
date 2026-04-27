#!/usr/bin/env sh
set -eu

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$repo_root/.git/hooks"
source_hook="$repo_root/.githooks/commit-msg"
target_hook="$hooks_dir/commit-msg"

if [ ! -f "$source_hook" ]; then
  echo "Missing hook source: $source_hook" >&2
  exit 1
fi

mkdir -p "$hooks_dir"
cp "$source_hook" "$target_hook"
chmod +x "$target_hook"

echo "Installed commit-msg hook at $target_hook"
