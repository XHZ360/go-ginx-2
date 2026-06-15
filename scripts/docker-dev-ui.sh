#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

pnpm --dir admin-ui install --frozen-lockfile
exec pnpm --dir admin-ui dev --host 0.0.0.0
