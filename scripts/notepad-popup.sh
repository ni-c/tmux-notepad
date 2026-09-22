#!/usr/bin/env bash
# Runs inside the tmux popup. Its whole job is to find the binary and exec it —
# and, the first time after a tpm install, to fetch it.
#
# The download happens here rather than in tmux-notepad.tmux because tpm runs
# that file synchronously while tmux is starting: a download there freezes the
# client for as long as the network takes. Here it sits inside a popup the user
# just opened on purpose, where waiting is visible and expected, and where tmux
# itself keeps running either way.
#
# exec, not a plain call: tmux-notepad reopens itself through os.Executable()
# when the popup is moved or resized, and that has to resolve to the binary
# rather than to this wrapper.

set -euo pipefail

NAME=tmux-notepad
REPO='ni-c/tmux-notepad'
PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$PLUGIN_DIR/bin/$NAME"
LOCK="$PLUGIN_DIR/.install.lock"

# Already installed here by a previous run: the common case, two syscalls, no
# network. Everything below this line happens exactly once per checkout.
[ -x "$BIN" ] && exec "$BIN" "$@"

# Installed system-wide instead — by the one-liner, `make install` or a package.
if command -v "$NAME" >/dev/null 2>&1; then
  exec "$NAME" "$@"
fi

# Everything below writes to stderr, so the message and this prompt come out in
# the order they were written rather than interleaved by two buffers.
pause() {
  # /dev/tty exists as a device node even where there is no terminal behind it,
  # so opening it is the only real test — and the failure has to be swallowed
  # here, or bash prints "No such device or address" of its own accord.
  if ! { exec 3</dev/tty; } 2>/dev/null; then
    return 0 # nothing to wait on: a pipe, a test, a runner
  fi
  printf '\nPress any key to close.' >&2
  { read -r -n 1 -s <&3 || read -r <&3 || true; } 2>/dev/null
  exec 3<&-
  printf '\n' >&2
}

# -E closes the popup the moment this script exits, so an error printed without
# a pause would flash past unread. Every failure path ends here.
fail() {
  printf '\n%s: %s\n' "$NAME" "$1" >&2
  printf '
Install it yourself instead:
  curl -fsSL https://github.com/%s/releases/latest/download/install.sh | bash
  go install github.com/%s/cmd/%s@latest
' "$REPO" "$REPO" "$NAME" >&2
  pause
  exit 1
}

# The release a plugin checkout belongs to. tpm checks out the default branch,
# not a tag, so the file on that branch names the newest published release; the
# release workflow refuses a tag that disagrees with it.
version=''
[ -r "$PLUGIN_DIR/VERSION" ] && version=$(tr -d '[:space:]' <"$PLUGIN_DIR/VERSION")

# Two panes can reach this point at the same time. The loser waits for the
# winner's binary rather than starting a second download into the same path.
if ! mkdir "$LOCK" 2>/dev/null; then
  printf '%s: another pane is installing it, waiting …\n' "$NAME" >&2
  for _ in $(seq 1 60); do
    sleep 1
    [ -x "$BIN" ] && exec "$BIN" "$@"
  done
  fail "timed out waiting for the other install. Remove $LOCK if nothing is running."
fi
trap 'rmdir "$LOCK" 2>/dev/null || true' EXIT

printf '%s: installing %s …\n' "$NAME" "${version:+version $version}" >&2

tmp=$(mktemp -d "${TMPDIR:-/tmp}/$NAME-boot.XXXXXX")
trap 'rm -rf "$tmp"; rmdir "$LOCK" 2>/dev/null || true' EXIT

if [ -n "$version" ]; then
  url="https://github.com/$REPO/releases/download/v$version/install.sh"
else
  # A checkout from before the first release, or one whose VERSION file was
  # removed. The newest release is the best guess available.
  url="https://github.com/$REPO/releases/latest/download/install.sh"
fi

if command -v curl >/dev/null 2>&1; then
  curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/install.sh" "$url" ||
    fail "download failed: $url"
elif command -v wget >/dev/null 2>&1; then
  wget -q --https-only -O "$tmp/install.sh" "$url" ||
    fail "download failed: $url"
else
  fail "neither curl nor wget is installed, so the binary cannot be fetched."
fi

# Into the plugin directory, not onto the PATH: tpm owns this checkout, and the
# key is already bound, so there is nothing for the installer to write to
# tmux.conf. The installer checks the archive against the checksums baked into
# the copy we just downloaded.
bash "$tmp/install.sh" --prefix "$PLUGIN_DIR" --no-bind --quiet ||
  fail "the installer did not finish."

[ -x "$BIN" ] || fail "the installer finished but left no binary at $BIN."

rm -rf "$tmp"
rmdir "$LOCK" 2>/dev/null || true
trap - EXIT
exec "$BIN" "$@"
