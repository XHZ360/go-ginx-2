#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mkdir -p .tmp/docker/bin .tmp/docker/admin-ui

if [[ ! -f .tmp/docker/admin-ui/index.html ]]; then
  cat > .tmp/docker/admin-ui/index.html <<'HTML'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>go-ginx dev backend</title>
  </head>
  <body>
    <p>Use the Vite Admin UI at http://localhost:5173 in Docker development.</p>
  </body>
</html>
HTML
fi

go build -o .tmp/docker/bin/goginx-server ./cmd/goginx-server
go build -o .tmp/docker/bin/goginx-admin ./cmd/goginx-admin
go build -o .tmp/docker/bin/goginx-client ./cmd/goginx-client

exec ./.tmp/docker/bin/goginx-server "$@"
