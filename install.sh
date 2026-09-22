#!/usr/bin/env bash
# Installs tmux-notepad (Linux, macOS, WSL).
#
#   curl -fsSL https://github.com/ni-c/tmux-notepad/releases/latest/download/install.sh | bash
#   … | bash -s -- --force        replace a `bind n` that belongs to something else
#   … | bash -s -- --no-bind      install the binary, leave tmux.conf alone
#   … | bash -s -- --uninstall    remove it again
#
# What it does: downloads the archive for this platform from the release this
# installer belongs to, checks it against the SHA-256 baked in below, puts the
# binary in ~/.local/bin, writes a starter config if there is none, and adds a
# marked key binding block to ~/.tmux.conf. Notes and config are never touched
# on uninstall — they are yours.
#
# Go is not needed. The binary is prebuilt.

set -euo pipefail

REPO='ni-c/tmux-notepad'
# Stamped by scripts/build-dist.sh when a release is built. An installer taken
# from a checkout has none of these and installs only from TMUX_NOTEPAD_DIST.
VERSION=''
# Read through indirect expansion further down, once the platform is known — a
# lookup the linter below cannot follow, hence the four disables.
# shellcheck disable=SC2034
SHA256_linux_amd64=''
# shellcheck disable=SC2034
SHA256_linux_arm64=''
# shellcheck disable=SC2034
SHA256_darwin_amd64=''
# shellcheck disable=SC2034
SHA256_darwin_arm64=''

NAME='tmux-notepad'
BEGIN_MARK="# >>> $NAME >>>"
END_MARK="# <<< $NAME <<<"

die() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  exit "${2:-1}"
}

QUIET=0
info() {
  [ "$QUIET" -eq 1 ] || printf '%s\n' "$1"
}

usage() {
  cat <<EOF
Usage: install.sh [--prefix DIR] [--force] [--no-bind] [--quiet] [--uninstall]

  --prefix DIR  install into DIR/bin instead of ~/.local/bin
  --force       replace a key binding that belongs to something else
  --no-bind     do not touch ~/.tmux.conf
  --quiet       only report problems
  --uninstall   remove the binary and our tmux.conf block
EOF
}

PREFIX=${PREFIX:-$HOME/.local}
FORCE=0 UNINSTALL=0 NO_BIND=0
while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) [ $# -ge 2 ] || die "--prefix needs a directory" 2; PREFIX=$2; shift 2 ;;
    --prefix=*) PREFIX=${1#--prefix=}; shift ;;
    --force) FORCE=1; shift ;;
    --no-bind) NO_BIND=1; shift ;;
    --quiet) QUIET=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; die "unknown option: $1" 2 ;;
  esac
done

BIN_DIR="$PREFIX/bin"
TARGET="$BIN_DIR/$NAME"
TMUX_CONF="${TMUX_CONF:-$HOME/.tmux.conf}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/$NAME"
CONFIG="$CONFIG_DIR/config.toml"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    openssl dgst -sha256 "$1" | sed 's/.*= //'
  fi
}

# Prints $1 with our marked block removed. Used both to take the block out on
# uninstall and to look for a foreign binding without tripping over our own.
without_block() {
  [ -e "$1" ] || return 0
  awk -v b="$BEGIN_MARK" -v e="$END_MARK" '
    $0 == b { skip = 1; next }
    skip && $0 == e { skip = 0; next }
    !skip
  ' "$1"
}

# Rewrites $1 with $2 as its content, through a temporary file so an interrupted
# run never leaves half a file. A symlinked tmux.conf (dotfile managers) is
# written through rather than replaced by a regular file.
replace_file() {
  local path=$1 content=$2 tmp="$1.tmp.$$"
  mkdir -p "$(dirname "$path")"
  printf '%s\n' "$content" >"$tmp"
  if [ -L "$path" ]; then
    cat "$tmp" >"$path"
    rm -f "$tmp"
  else
    mv -f "$tmp" "$path"
  fi
}

if [ "$UNINSTALL" -eq 1 ]; then
  if [ -e "$TARGET" ]; then
    rm -f "$TARGET"
    info "Removed $TARGET"
  fi
  if [ -e "$TMUX_CONF" ] && grep -qF "$BEGIN_MARK" "$TMUX_CONF"; then
    cp -p "$TMUX_CONF" "$TMUX_CONF.before-$NAME"
    replace_file "$TMUX_CONF" "$(without_block "$TMUX_CONF")"
    info "Removed the $NAME block from $TMUX_CONF (previous version saved as $TMUX_CONF.before-$NAME)"
    if tmux has-session >/dev/null 2>&1; then
      tmux source-file "$TMUX_CONF" >/dev/null 2>&1 || true
    fi
  fi
  info "Left your notes and $CONFIG alone — those are yours to remove."
  exit 0
fi

case "$(uname -s 2>/dev/null)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  MINGW* | MSYS* | CYGWIN*)
    die "tmux does not run natively on Windows. Install this inside WSL instead." ;;
  *) die "unsupported system: $(uname -s). Build from source with: go install github.com/$REPO/cmd/$NAME@latest" ;;
esac

case "$(uname -m 2>/dev/null)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m). Build from source with: go install github.com/$REPO/cmd/$NAME@latest" ;;
esac

archive="${NAME}_${VERSION}_${os}_${arch}.tar.gz"
eval "expected=\${SHA256_${os}_${arch}}"

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/$NAME.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT
tmp_archive="$tmp_dir/archive.tar.gz"

if [ -n "${TMUX_NOTEPAD_DIST:-}" ]; then
  # Escape hatch for testing a build before it is a release, and for the CI job
  # that installs from the dist/ it just built.
  cp "$TMUX_NOTEPAD_DIST/$archive" "$tmp_archive"
elif [ -z "$VERSION" ]; then
  die "this installer does not belong to a release. Use the one-liner from the README,
or set TMUX_NOTEPAD_DIST to a directory of built archives to install from there."
else
  url="https://github.com/$REPO/releases/download/v$VERSION/$archive"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp_archive" "$url" || die "download failed: $url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --https-only -O "$tmp_archive" "$url" || die "download failed: $url"
  else
    die "curl or wget is required to download $url"
  fi
fi

if [ -n "$expected" ]; then
  actual=$(sha256_of "$tmp_archive")
  [ "$actual" = "$expected" ] ||
    die "checksum mismatch for $archive: expected $expected, got $actual. Nothing was changed."
fi

tar -xzf "$tmp_archive" -C "$tmp_dir"
unpacked="$tmp_dir/${NAME}_${VERSION}_${os}_${arch}"
[ -d "$unpacked" ] || unpacked=$(find "$tmp_dir" -mindepth 1 -maxdepth 1 -type d | head -n1)
[ -x "$unpacked/$NAME" ] || die "the archive does not contain a $NAME binary"

# Moved into place rather than copied, so an interrupted install never leaves a
# half-written binary where a working one used to be.
mkdir -p "$BIN_DIR"
mv -f "$unpacked/$NAME" "$TARGET.new.$$"
chmod 755 "$TARGET.new.$$"
mv -f "$TARGET.new.$$" "$TARGET"

# Proves the binary matches this machine and runs at all, before claiming success.
installed_version=$("$TARGET" --version 2>&1) || die "installed, but it does not run: $installed_version"
if [ -n "$VERSION" ] && [ "$installed_version" != "$NAME $VERSION" ]; then
  die "installed, but it reports '$installed_version' instead of '$NAME $VERSION'"
fi

if [ ! -e "$CONFIG" ] && [ -e "$unpacked/config.example.toml" ]; then
  mkdir -p "$CONFIG_DIR"
  cp "$unpacked/config.example.toml" "$CONFIG"
  info "Wrote a starter config to $CONFIG — point it at your notes."
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) info "Note: $BIN_DIR is not on your PATH. Add it:
  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

if [ "$NO_BIND" -eq 0 ]; then
  key=n
  block="$BEGIN_MARK
bind $key display-popup -E -w 80% -h 80% -T ' notepad ' '$NAME'
$END_MARK"
  rest=$(without_block "$TMUX_CONF")
  # A binding for the same key that is not ours. Matches `bind n`, `bind-key n`
  # and the flag forms in between, so we do not quietly shadow someone's binding.
  if printf '%s\n' "$rest" |
    grep -qE "^[[:space:]]*bind(-key)?([[:space:]]+-[^[:space:]]+)*[[:space:]]+$key([[:space:]]|\$)"; then
    [ "$FORCE" -eq 1 ] || die "$TMUX_CONF already binds '$key' to something else.
Re-run with --force to replace it, or with --no-bind and bind it yourself:
  bind <key> display-popup -E -w 80% -h 80% -T ' notepad ' '$NAME'"
    # || true: grep exits 1 when it filters everything out, which is exactly
    # what happens to a tmux.conf whose only line is the binding we replace.
    rest=$(printf '%s\n' "$rest" |
      grep -vE "^[[:space:]]*bind(-key)?([[:space:]]+-[^[:space:]]+)*[[:space:]]+$key([[:space:]]|\$)" || true)
  fi
  if [ -e "$TMUX_CONF" ]; then
    cp -p "$TMUX_CONF" "$TMUX_CONF.before-$NAME"
  fi
  replace_file "$TMUX_CONF" "${rest:+$rest
}$block"
  info "Bound prefix+$key in $TMUX_CONF"
  if tmux has-session >/dev/null 2>&1; then
    if tmux source-file "$TMUX_CONF" >/dev/null 2>&1; then
      info "Reloaded your running tmux, so prefix+$key works now."
    fi
  fi
fi

info "Installed $installed_version to $TARGET"
