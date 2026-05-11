# Admin CLI SQLite Seed Example

The `goginx-admin` command seeds milestone-one resources into SQLite. It is non-interactive and prints the created resource ID to stdout.

Create a temporary data directory first:

```powershell
New-Item -ItemType Directory -Force .tmp
```

Create a user:

```powershell
go run ./cmd/goginx-admin create-user `
  -db ./.tmp/go-ginx.db `
  -id user-1 `
  -username alice
```

Create a client credential:

```powershell
go run ./cmd/goginx-admin create-client `
  -db ./.tmp/go-ginx.db `
  -id client-1 `
  -user user-1 `
  -name home `
  -credential secret
```

Create a TCP proxy:

```powershell
go run ./cmd/goginx-admin create-tcp-proxy `
  -db ./.tmp/go-ginx.db `
  -id tcp-1 `
  -user user-1 `
  -client client-1 `
  -name ssh `
  -port 10022 `
  -target-host 127.0.0.1 `
  -target-port 22
```

Create an HTTP proxy:

```powershell
go run ./cmd/goginx-admin create-http-proxy `
  -db ./.tmp/go-ginx.db `
  -id web-1 `
  -user user-1 `
  -client client-1 `
  -name web `
  -host app.example.com `
  -target-host 127.0.0.1 `
  -target-port 8080
```

The current server/client binaries do not yet consume this database in daemon mode. The database is used by package-level runtime tests today and will become the seed source for the daemon wiring step.
