#!/bin/sh

set -eu

install_dir=${DOCKET_INSTALL_DIR:-"$HOME/.local/bin"}
release_url=${DOCKET_RELEASE_URL:-https://github.com/glamboyosa/docket/releases/latest/download}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *)
    echo "docket: unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)
    echo "docket: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

for command_name in curl tar awk mktemp; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "docket: $command_name is required" >&2
    exit 1
  fi
done

archive="docket_${os}_${arch}.tar.gz"
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/docket-install.XXXXXX")
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

curl -fsSL "$release_url/$archive" -o "$temporary_dir/$archive"
curl -fsSL "$release_url/checksums.txt" -o "$temporary_dir/checksums.txt"

expected_checksum=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$temporary_dir/checksums.txt")
if [ -z "$expected_checksum" ]; then
  echo "docket: $archive is missing from checksums.txt" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum=$(sha256sum "$temporary_dir/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum=$(shasum -a 256 "$temporary_dir/$archive" | awk '{ print $1 }')
else
  echo "docket: sha256sum or shasum is required" >&2
  exit 1
fi

if [ "$actual_checksum" != "$expected_checksum" ]; then
  echo "docket: checksum verification failed for $archive" >&2
  exit 1
fi

tar -xzf "$temporary_dir/$archive" -C "$temporary_dir"
mkdir -p "$install_dir"
cp "$temporary_dir/docket" "$install_dir/docket"
chmod 0755 "$install_dir/docket"

echo "Installed docket to $install_dir/docket"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "$install_dir is not on PATH. Add this line to your shell profile:"
    echo "  export PATH=\"$install_dir:\$PATH\""
    ;;
esac
