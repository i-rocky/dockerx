# dockerx

`dockerx` launches a hardened dev container in your current directory:

- Your current directory is mounted at `/app` as read-write.
- The container root filesystem is read-only.
- Host config paths are bind-mounted read-only under `/tmp`, copied into `~/...` on startup, and diffed on exit.
- The goal is host safety: strong container access with minimal host write surface.

## Installation

### Windows (Scoop)

```powershell
scoop bucket add rocky https://github.com/i-rocky/scoop-bucket
scoop install dockerx
```

### macOS/Linux (Homebrew)

```sh
brew tap i-rocky/tap
brew install dockerx
```

### Manual (GitHub Releases)

Download the platform archive from the latest release, extract it, and put
`dockerx` on your `PATH`.

## Quick start

```sh
dockerx
dockerx -- make test
dockerx --image wpkpda/dockerx:latest
```

## CLI flags

- `--image`: container image (default `wpkpda/dockerx:latest` or `DOCKERX_IMAGE`)
- `--no-pull`: disable forced `--pull always` for `wpkpda/dockerx` images (useful for local `:test` tags)
- `--shell`: shell when no command is provided (default `zsh`)
- `--no-config`: disable automatic host config mounts
- `--dry-run`: print docker command without running it
- `--verbose`: print resolved mounts/env passthrough
- `--version`: print binary version

## Security defaults

`dockerx` starts the container with:

- `--read-only`
- `--cap-drop ALL` with minimal adds: `SETUID`, `SETGID`, `AUDIT_WRITE` (to support `sudo`)
- `/app` bind-mounted read-write
- config mounts bind-mounted read-only under `/tmp`, then copied into home paths
- runtime identity overlays for `/etc/passwd`, `/etc/group`, `/etc/shadow` so host-mapped UID/GID is resolvable by setuid tools like `sudo`
- tmpfs mounts for `/tmp`, `/run`, `/var/tmp`, `/var/lib/apt/lists`, `/var/cache/apt`, and container home

## Build

```sh
go build -o dockerx .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dockerx.exe .
```

## Scoop and Homebrew

Release tags (`v*`) build platform artifacts and publish:

- Windows zip for Scoop
- macOS/Linux tarballs and checksums for Homebrew formulas

See `.github/workflows/release.yml`.
