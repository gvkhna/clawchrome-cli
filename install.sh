#!/bin/sh
set -eu

REPO_OWNER="gvkhna"
REPO_NAME="clawchrome-cli"
BIN_DIR="${HOME}/.local/bin"
VERSION=""

usage() {
  cat <<EOF
usage: install.sh [--version vX.Y.Z] [--bin-dir PATH]

Install clawchrome-cli by downloading the raw release binary from GitHub Releases.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *)
      echo "unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi
  echo "curl or wget is required" >&2
  exit 1
}

latest_version() {
  tmp="$(mktemp)"
  download "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" "$tmp"
  version="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp" | head -n 1)"
  rm -f "$tmp"
  if [ -z "$version" ]; then
    echo "failed to resolve latest version" >&2
    exit 1
  fi
  printf '%s\n' "$version"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  echo "sha256sum or shasum is required" >&2
  exit 1
}

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [ -z "$VERSION" ]; then
  VERSION="$(latest_version)"
fi

case "$VERSION" in
  v*) ;;
  *) VERSION="v${VERSION}" ;;
esac

ASSET="${REPO_NAME}_${VERSION#v}_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  ASSET="${ASSET}.exe"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

BINARY_PATH="${TMP_DIR}/${ASSET}"
CHECKSUMS_PATH="${TMP_DIR}/checksums.txt"

download "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/${ASSET}" "$BINARY_PATH"
download "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}/checksums.txt" "$CHECKSUMS_PATH"

EXPECTED="$(awk -v asset="${ASSET}" '$2 == asset || $2 == "*" asset { print $1 }' "$CHECKSUMS_PATH" | head -n 1)"
if [ -z "$EXPECTED" ]; then
  echo "checksum entry not found for ${ASSET}" >&2
  exit 1
fi

ACTUAL="$(sha256_file "$BINARY_PATH")"
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "checksum mismatch for ${ASSET}" >&2
  exit 1
fi

mkdir -p "$BIN_DIR"
TARGET="${BIN_DIR}/clawchrome-cli"
if [ "$OS" = "windows" ]; then
  TARGET="${TARGET}.exe"
fi
cp "$BINARY_PATH" "$TARGET"
chmod 0755 "$TARGET"

echo "installed ${TARGET}"
case ":$PATH:" in
  *":${BIN_DIR}:"*) ;;
  *)
    echo "note: ${BIN_DIR} is not in PATH" >&2
    ;;
esac
