# Go toolchain

MEKU-In targets the Go 1.24 release line and pins the reproducible toolchain to **Go 1.24.13** (the latest patch in that line at the time this environment was prepared).

## Install on Windows (PowerShell)

From the repository root:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\install-go-1.24.13.ps1
```

Restart the terminal, then verify:

```powershell
go version
mise install
```

## Install on Linux/macOS

```sh
sh scripts/install-go-1.24.13.sh
```

Add the printed Go `bin` directory to `PATH`, restart the shell, then run `mise install`.

## Resolve and lock dependencies

The first network-enabled Go 1.24.13 environment must run:

```sh
cd services/api
go mod tidy
go mod verify
go test ./...
go vet ./...
go build ./cmd/api
```

Commit the generated `go.sum`. The Docker build intentionally copies both `go.mod` and `go.sum`, so image builds fail rather than silently omit dependency checksums.

## Why this patch version

Use the latest Go 1.24 patch rather than the original 1.24.0 toolchain. This keeps the requested 1.24 compatibility while including fixes released later in that branch. CI and the container build use the same exact version.
