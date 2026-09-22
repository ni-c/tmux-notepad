#!/usr/bin/env bash
# Builds the release assets into a directory:
#
#   scripts/build-dist.sh 1.2.3 dist
#
# One tar.gz per platform, plus install.sh with the version and the checksum of
# every archive baked in — so a release installer only ever installs the exact
# bytes released with it. SHA256SUMS covers all assets for anyone checking by hand.
#
# tmux does not run natively on Windows, so there is no Windows build; WSL uses
# the linux one. The archives are packed reproducibly, which needs GNU tar — this
# script is meant for the Linux release runner, not for macOS.
set -euo pipefail

version=${1:?usage: build-dist.sh VERSION OUTDIR}
out=${2:?usage: build-dist.sh VERSION OUTDIR}
root=$(cd "$(dirname "$0")/.." && pwd)

case "$version" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "not a version: $version" >&2; exit 2 ;;
esac

NAME=tmux-notepad
PLATFORMS='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64'

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# stamp FILE PATTERN REPLACEMENT — exactly one line must match, or the build fails.
stamp() {
  local count
  count=$(grep -c "$2" "$1" || true)
  [ "$count" = 1 ] || { echo "$1: expected one line matching '$2', found $count" >&2; exit 1; }
  sed "s|$2|$3|" "$1" >"$1.tmp" && mv "$1.tmp" "$1"
}

rm -rf "$out"
mkdir -p "$out"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

for platform in $PLATFORMS; do
  os=${platform%/*}
  arch=${platform#*/}
  dir="${NAME}_${version}_${os}_${arch}"
  mkdir -p "$stage/$dir"

  # -trimpath and a cleared buildid keep the binary free of build-host paths, so
  # two runs of this script produce the same bytes. CGO off makes one linux
  # binary that works on glibc and musl alike.
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -buildid= -X main.version=$version" \
    -o "$stage/$dir/$NAME" "$root/cmd/$NAME"

  cp "$root/LICENSE" "$root/README.md" "$root/CHANGELOG.md" \
     "$root/config.example.toml" "$root/tmux-notepad.tmux" "$stage/$dir/"

  # --sort, --mtime, --owner/--group and gzip -n strip everything that would
  # otherwise differ between two builds of the same source.
  tar --sort=name --mtime="@${SOURCE_DATE_EPOCH:-0}" \
      --owner=0 --group=0 --numeric-owner \
      -cf - -C "$stage" "$dir" | gzip -9n >"$out/$dir.tar.gz"
done

cp "$root/install.sh" "$out/"
stamp "$out/install.sh" "^VERSION=''\$" "VERSION='$version'"
for platform in $PLATFORMS; do
  os=${platform%/*}
  arch=${platform#*/}
  sum=$(sha256_of "$out/${NAME}_${version}_${os}_${arch}.tar.gz")
  stamp "$out/install.sh" "^SHA256_${os}_${arch}=''\$" "SHA256_${os}_${arch}='$sum'"
done
chmod 755 "$out/install.sh"

# Last, so it covers the stamped installer too. Collected into a variable and
# written in one go: a redirection on the loop would create SHA256SUMS before
# the glob expands, and the file would end up listing itself.
sums=''
for f in "$out"/*; do
  sums="$sums$(sha256_of "$f")  $(basename "$f")
"
done
printf '%s' "$sums" >"$out/SHA256SUMS"
