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

Create a UDP proxy:

```powershell
go run ./cmd/goginx-admin create-udp-proxy `
  -db ./.tmp/go-ginx.db `
  -id udp-1 `
  -user user-1 `
  -client client-1 `
  -name dns `
  -port 10053 `
  -target-host 127.0.0.1 `
  -target-port 53
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

Create an HTTPS passthrough proxy:

```powershell
go run ./cmd/goginx-admin create-https-proxy `
  -db ./.tmp/go-ginx.db `
  -id secure-1 `
  -user user-1 `
  -client client-1 `
  -name secure `
  -host secure.example.com `
  -target-host 127.0.0.1 `
  -target-port 8443
```

Create an HTTPS termination proxy. The public server selects this certificate by SNI, terminates TLS, and forwards the decrypted HTTP request to the configured local HTTP target:

```powershell
go run ./cmd/goginx-admin create-https-proxy `
  -db ./.tmp/go-ginx.db `
  -id secure-term-1 `
  -user user-1 `
  -client client-1 `
  -name secure-term `
  -host term.example.com `
  -target-host 127.0.0.1 `
  -target-port 8080 `
  -cert-file data/certs/term.crt `
  -key-file data/certs/term.key
```

Daemon mode now consumes this seeded SQLite database. Start `goginx-server` with a `sqlite_path` that points at this file, then start `goginx-client` with the seeded `client_id` and credential.
