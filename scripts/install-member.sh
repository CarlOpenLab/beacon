#!/usr/bin/env bash
# Install Beacon's member daemon and register it as this machine's service.
#
#   curl -fsSL https://raw.githubusercontent.com/CarlOpenLab/beacon/main/scripts/install-member.sh | \
#     sh -s -- -relay wss://relay.example/relay/agent -device-id <id> -device-secret <secret>
#
# Run it again to update: the binary is replaced and the service restarted, and
# nothing about the enrollment changes. The flags after `--` are the flags the
# service will run with.
set -euo pipefail

repo="CarlOpenLab/beacon"
bin_dir="${BEACON_BIN_DIR:-$HOME/.local/bin}"

os="$(uname -s)"
case "$os" in
  Darwin) goos="darwin" ;;
  Linux) goos="linux" ;;
  *) echo "install-member: unsupported system $os (use the .zip on Windows)" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *) echo "install-member: unsupported architecture $arch" >&2; exit 1 ;;
esac

command -v curl >/dev/null || { echo "install-member: curl is required" >&2; exit 1; }

# The asset name carries the version, so the tag has to be known before the
# download. The API is rate limited for anonymous callers but generous enough
# for an install.
tag="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" |
  sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
[ -n "$tag" ] || { echo "install-member: cannot find the latest release" >&2; exit 1; }
version="${tag#v}"

asset="beacon-cli_${version}_${goos}_${goarch}.tar.gz"
url="https://github.com/${repo}/releases/download/${tag}/${asset}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading ${tag} (${goos}/${goarch})…"
curl -fsSL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

mkdir -p "$bin_dir"
install -m 0755 "$tmp/tfd" "$bin_dir/tfd"

echo "installed $bin_dir/tfd ($("$bin_dir/tfd" version))"

# The flags after this script's own `--` are the service's flags. Installing
# before the first run means there is no daemon to stop, and re-running it
# replaces the binary under a service that is then restarted.
if ! "$bin_dir/tfd" service install "$@"; then
  echo >&2
  echo "install-member: the binary is installed but the service was not. Run:" >&2
  echo "  $bin_dir/tfd service install $*" >&2
  exit 1
fi
