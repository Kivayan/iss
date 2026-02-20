# ISS Terminal App

Animated terminal app that renders an ASCII ISS and live telemetry (`ISS over: {country}`).

## Quick install

Install from GitHub Releases with one command.

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/kivayan/iss/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/kivayan/iss/main/scripts/install.ps1 | iex
```

The installers:

- detect your OS/arch,
- download the matching release binary,
- verify SHA256 using `checksums.txt`,
- install `iss` into a user-local directory.

After install, run:

```bash
iss
```

Quit with `q` or `ctrl+c`.

## Install a specific version

### Linux

```bash
curl -fsSL https://raw.githubusercontent.com/kivayan/iss/main/scripts/install.sh | bash -s -- --version v0.1.0
```

### Windows (PowerShell)

```powershell
$env:ISS_VERSION = "v0.1.0"; irm https://raw.githubusercontent.com/kivayan/iss/main/scripts/install.ps1 | iex
```

## Installer configuration

Both installers support:

- `ISS_REPO` (default: `kivayan/iss`)
- `ISS_VERSION` (default: `latest`, accepts `v1.2.3` or `1.2.3`)
- `ISS_INSTALL_DIR` (custom install location)

Example (Linux, custom install dir):

```bash
ISS_INSTALL_DIR="$HOME/bin" curl -fsSL https://raw.githubusercontent.com/kivayan/iss/main/scripts/install.sh | bash
```

## Run from source

```bash
go run .
```

## Releases (maintainers)

Yes, this uses **git tags**.

Creating and pushing a tag like `v0.1.0` triggers the release workflow in `.github/workflows/release.yml`.

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

The workflow publishes:

- `iss_linux_amd64.tar.gz`
- `iss_linux_arm64.tar.gz`
- `iss_windows_amd64.zip`
- `iss_windows_arm64.zip`
- `checksums.txt`
