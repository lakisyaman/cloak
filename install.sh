#!/bin/sh
# Cloak installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/lakisyaman/cloak/main/install.sh | sh -s -- v0.1.0
#
# Environment:
#   VERSION            release to install (default: latest), e.g. v0.1.0
#   CLOAK_INSTALL_DIR  install directory (default: $HOME/.local/bin)
#   CLOAK_BASE_URL     override the release download base URL (mirrors/testing)

set -eu

REPO="lakisyaman/cloak"
BINARY="cloak"
INSTALL_DIR="${CLOAK_INSTALL_DIR:-$HOME/.local/bin}"

error() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

have() {
	command -v "$1" >/dev/null 2>&1
}

# download URL OUTFILE
download() {
	if have curl; then
		curl -fsSL "$1" -o "$2"
	elif have wget; then
		wget -qO "$2" "$1"
	else
		error "curl or wget is required"
	fi
}

# fetch URL (to stdout)
fetch() {
	if have curl; then
		curl -fsSL "$1"
	elif have wget; then
		wget -qO- "$1"
	else
		error "curl or wget is required"
	fi
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
	linux) os=linux ;;
	darwin) os=darwin ;;
	*) error "unsupported operating system: $os (Cloak supports linux and darwin)" ;;
esac

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) error "unsupported architecture: $arch (Cloak supports amd64 and arm64)" ;;
esac

# Resolve version: explicit arg/env wins, else the latest published release.
tag="${VERSION:-${1:-}}"
if [ -z "$tag" ]; then
	tag=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
		grep '"tag_name":' | head -1 |
		sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
fi
[ -n "$tag" ] || error "could not determine the latest release; pass a version explicitly"

version="${tag#v}"
tag="v$version"

base_url="${CLOAK_BASE_URL:-https://github.com/$REPO/releases/download/$tag}"
archive="${BINARY}_${version}_${os}_${arch}.tar.gz"

printf 'Installing %s %s (%s/%s)\n' "$BINARY" "$tag" "$os" "$arch"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

download "$base_url/$archive" "$tmp/$archive"
download "$base_url/checksums.txt" "$tmp/checksums.txt"

expected=$(awk -v f="$archive" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$expected" ] || error "checksum for $archive not found in checksums.txt"
if have sha256sum; then
	actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif have shasum; then
	actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
	error "sha256sum or shasum is required to verify the download"
fi
[ "$expected" = "$actual" ] || error "checksum mismatch for $archive (expected $expected, got $actual)"

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BINARY" ] || error "archive did not contain a $BINARY binary"
mkdir -p "$INSTALL_DIR"
cp "$tmp/$BINARY" "$INSTALL_DIR/$BINARY"
chmod 0755 "$INSTALL_DIR/$BINARY"

printf '\nInstalled %s to %s\n' "$BINARY" "$INSTALL_DIR/$BINARY"

case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		printf '\n%s is not on your PATH. Add it:\n\n    export PATH="%s:$PATH"\n' \
			"$INSTALL_DIR" "$INSTALL_DIR"
		;;
esac

cat <<EOF

Next steps:
  1. Install a shim:     $BINARY shim install psql
  2. Put shims on PATH:  export PATH="\$($BINARY shim dir):\$PATH"
  3. Enroll a context:   psql cloak context add production --host ... --username ... --password ...
  4. Switch and use:     psql cloak context switch production && psql
  5. Diagnose anytime:   $BINARY doctor
EOF
