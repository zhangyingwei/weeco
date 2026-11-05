#!/usr/bin/env bash
set -euo pipefail

REPO="${WEECO_REPO:-zhangyingwei/weeco}"
VERSION="${WEECO_VERSION:-0.12.2}"
VERSION="${VERSION#v}"
BIN_DIR="${WEECO_BIN_DIR:-$HOME/.local/bin}"
case "$BIN_DIR" in
  "~") BIN_DIR="$HOME" ;;
  "~/"*) BIN_DIR="$HOME/${BIN_DIR#\~/}" ;;
esac
BASE_URL="${WEECO_BASE_URL:-https://github.com/${REPO}/releases/download}"
BASE_URL="${BASE_URL%/}"

die() {
  printf 'weeco install: %s\n' "$*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1
}

download() {
  local url="$1" dest="$2"
  if need curl; then
    curl --proto '=https' --tlsv1.2 -fsSL "$url" -o "$dest"
  elif need wget; then
    wget -qO "$dest" "$url"
  else
    die "curl or wget is required"
  fi
}

download_stdout() {
  local url="$1"
  if need curl; then
    curl --proto '=https' --tlsv1.2 -fsSL "$url"
  elif need wget; then
    wget -qO- "$url"
  else
    die "curl or wget is required"
  fi
}

if [ "$VERSION" = "latest" ]; then
  VERSION="$(download_stdout "https://api.github.com/repos/${REPO}/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\{0,1\}\([^"]*\)".*/\1/p' |
    head -n 1)"
  [ -n "$VERSION" ] || die "could not resolve the latest release"
fi

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

asset="weeco-${VERSION}-${os}-${arch}.tar.gz"
release_url="${BASE_URL}/v${VERSION}"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/weeco-install.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

printf 'weeco install: downloading %s\n' "$asset"
download "${release_url}/SHA256SUMS" "${tmp}/SHA256SUMS"
download "${release_url}/${asset}" "${tmp}/${asset}"

checksum_line="$(awk -v asset="$asset" '$2 == asset { print; exit }' "${tmp}/SHA256SUMS")"
[ -n "$checksum_line" ] || die "SHA256SUMS has no entry for ${asset}"
printf '%s\n' "$checksum_line" > "${tmp}/${asset}.sha256"
(
  cd "$tmp"
  if need sha256sum; then
    sha256sum -c "${asset}.sha256"
  elif need shasum; then
    shasum -a 256 -c "${asset}.sha256"
  else
    die "sha256sum or shasum is required"
  fi
)

tar -xzf "${tmp}/${asset}" -C "$tmp"
[ -f "${tmp}/weeco" ] || die "release archive did not contain weeco"
mkdir -p "$BIN_DIR" || die "cannot create ${BIN_DIR}"
if need install; then
  install -m 0755 "${tmp}/weeco" "${BIN_DIR}/weeco"
else
  cp "${tmp}/weeco" "${BIN_DIR}/weeco"
  chmod 0755 "${BIN_DIR}/weeco"
fi

"${BIN_DIR}/weeco" install --bin-dir "$BIN_DIR"

printf '\nweeco install: binary %s\n' "${BIN_DIR}/weeco"
printf 'weeco install: version %s\n' "$("${BIN_DIR}/weeco" version)"

missing=0
for dep in herdr claude; do
  if need "$dep"; then
    printf 'weeco install: found %s\n' "$dep"
  else
    printf 'weeco install: warning: %s is not on PATH\n' "$dep" >&2
    missing=1
  fi
done

case ":${PATH}:" in
  *":${BIN_DIR}:"*) ;;
  *) printf 'weeco install: add %s to PATH before invoking weeco\n' "$BIN_DIR" ;;
esac

if [ "$missing" -ne 0 ]; then
  printf 'weeco install: install Herdr and Claude Code, complete their login, then run /weeco in a Herdr pane\n' >&2
  if [ "${WEECO_REQUIRE_DEPS:-0}" = "1" ]; then
    exit 1
  fi
fi

printf 'weeco install: done\n'
