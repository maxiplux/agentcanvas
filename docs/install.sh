#!/bin/sh
# AgentCanvas installer for macOS and Linux.
#
#   curl -fsSL https://maxiplux.github.io/agentcanvas/install.sh | sh
#
# Installs the latest agent-canvas release, verified against SHA256SUMS. Run it
# again at any time to update; `agent-canvas update` does the same from the CLI.
# Never prompts and never needs sudo, so coding agents can run it unattended.
#
# Environment (all optional):
#   AGENT_CANVAS_VERSION      release tag to install, e.g. v0.1.1 (default: latest)
#   AGENT_CANVAS_INSTALL_DIR  directory for the binary (default: the directory of an
#                             existing agent-canvas, else /usr/local/bin if writable,
#                             else ~/.local/bin)
set -eu

REPO="maxiplux/agentcanvas"
VERSION="${AGENT_CANVAS_VERSION:-latest}"

say() { printf '%s\n' "agent-canvas installer: $*"; }
fail() { printf '%s\n' "agent-canvas installer: error: $*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported OS $(uname -s); on Windows run: irm https://maxiplux.github.io/agentcanvas/install.ps1 | iex" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported CPU $(uname -m) (supported: x86_64, arm64)" ;;
esac
asset="agent-canvas-$os-$arch"

if [ "$VERSION" = latest ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi

fetch() { # fetch URL FILE
  if command -v curl > /dev/null 2>&1; then
    curl -fsSL --retry 3 -o "$2" "$1"
  elif command -v wget > /dev/null 2>&1; then
    wget -q -O "$2" "$1"
  else
    fail "need curl or wget"
  fi
}

sha256() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    fail "need sha256sum or shasum to verify the download"
  fi
}

tmp="$(mktemp -d 2> /dev/null || mktemp -d -t agent-canvas)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading $asset ($VERSION)"
fetch "$base/$asset" "$tmp/$asset" || fail "download failed: $base/$asset"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"

want="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")"
got="$(sha256 "$tmp/$asset")"
[ -n "$want" ] || fail "$asset is not listed in SHA256SUMS"
[ "$want" = "$got" ] || fail "checksum mismatch for $asset (want $want, got $got)"
chmod 0755 "$tmp/$asset"

dir="${AGENT_CANVAS_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  existing="$(command -v agent-canvas 2> /dev/null || true)"
  if [ -n "$existing" ] && [ -w "$(dirname "$existing")" ]; then
    dir="$(dirname "$existing")"
  elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir" || fail "cannot create $dir"
[ -w "$dir" ] || fail "$dir is not writable; set AGENT_CANVAS_INSTALL_DIR to a writable directory"

# copy next to the target, then rename: a running agent-canvas keeps working
cp "$tmp/$asset" "$dir/.agent-canvas.new"
mv -f "$dir/.agent-canvas.new" "$dir/agent-canvas"
if [ "$os" = darwin ] && command -v xattr > /dev/null 2>&1; then
  xattr -d com.apple.quarantine "$dir/agent-canvas" 2> /dev/null || true
fi

installed="$("$dir/agent-canvas" version 2> /dev/null || echo "agent-canvas")"
say "installed $installed to $dir/agent-canvas"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "add it to your PATH:  export PATH=\"$dir:\$PATH\"" ;;
esac
say "set up a project for your coding agent with: cd your-project && agent-canvas init"
say "open the studio with: agent-canvas open"
