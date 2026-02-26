#!/usr/bin/env bash

set -euo pipefail

tool_name="iss"
repo_slug="${ISS_REPO:-kivayan/iss}"
version="${ISS_VERSION:-latest}"
install_dir="${ISS_INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<'EOF'
Install iss from GitHub Releases.

Options:
  --repo <owner/repo>      Repository slug (default: ISS_REPO or kivayan/iss)
  --version <version>      Release version, ex: v1.2.3 or 1.2.3 (default: latest)
  --install-dir <path>     Install directory (default: ISS_INSTALL_DIR or ~/.local/bin)
  -h, --help               Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      if [[ $# -lt 2 ]]; then
        printf 'error: --repo requires a value\n' >&2
        exit 1
      fi
      repo_slug="$2"
      shift 2
      ;;
    --version)
      if [[ $# -lt 2 ]]; then
        printf 'error: --version requires a value\n' >&2
        exit 1
      fi
      version="$2"
      shift 2
      ;;
    --install-dir)
      if [[ $# -lt 2 ]]; then
        printf 'error: --install-dir requires a value\n' >&2
        exit 1
      fi
      install_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'error: unknown argument %s\n' "$1" >&2
      usage
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command %s was not found\n' "$1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd tar
need_cmd awk

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
    return
  fi

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
    return
  fi

  printf 'error: required command sha256sum or shasum was not found\n' >&2
  exit 1
}

os_name="$(uname -s)"
case "$os_name" in
  Linux)
    os="linux"
    ;;
  Darwin)
    os="darwin"
    ;;
  *)
    printf 'error: this installer supports Linux and macOS only (detected %s)\n' "$os_name" >&2
    exit 1
    ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64)
    arch="amd64"
    ;;
  aarch64|arm64)
    arch="arm64"
    ;;
  *)
    printf 'error: unsupported architecture %s\n' "$machine" >&2
    exit 1
    ;;
esac

if [[ "$version" != "latest" && "$version" != v* ]]; then
  version="v${version}"
fi

asset_name="${tool_name}_${os}_${arch}.tar.gz"
checksums_name="checksums.txt"

if [[ "$version" == "latest" ]]; then
  base_url="https://github.com/${repo_slug}/releases/latest/download"
else
  base_url="https://github.com/${repo_slug}/releases/download/${version}"
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

archive_path="${tmp_dir}/${asset_name}"
checksums_path="${tmp_dir}/${checksums_name}"

printf 'Downloading %s from %s\n' "$asset_name" "$repo_slug"
curl -fsSL "${base_url}/${asset_name}" -o "$archive_path"
curl -fsSL "${base_url}/${checksums_name}" -o "$checksums_path"

expected_checksum="$(awk -v file="$asset_name" '$2 == file { print $1 }' "$checksums_path")"
if [[ -z "$expected_checksum" ]]; then
  printf 'error: checksum for %s not found in %s\n' "$asset_name" "$checksums_name" >&2
  exit 1
fi

actual_checksum="$(sha256_file "$archive_path")"
if [[ "$expected_checksum" != "$actual_checksum" ]]; then
  printf 'error: checksum mismatch for %s\n' "$asset_name" >&2
  exit 1
fi

tar -xzf "$archive_path" -C "$tmp_dir"

binary_path="${tmp_dir}/${tool_name}"
if [[ ! -f "$binary_path" ]]; then
  printf 'error: expected binary %s not found in release archive\n' "$tool_name" >&2
  exit 1
fi

mkdir -p "$install_dir"
target_path="${install_dir}/${tool_name}"
cp "$binary_path" "$target_path"
chmod 0755 "$target_path"

printf 'Installed %s to %s\n' "$tool_name" "$target_path"

if [[ ":$PATH:" != *":${install_dir}:"* ]]; then
  printf '\n%s is not currently in PATH. Add this line to your shell profile:\n' "$install_dir"
  printf '  export PATH="%s:$PATH"\n' "$install_dir"
fi

printf '\nDone. Run `%s --help` to verify.\n' "$tool_name"
