#!/usr/bin/env bash
# tpm entry point: binds the key that opens the notepad popup.
#
# display-popup is used directly rather than through run-shell: run-shell -b
# has no client and so cannot show a popup at all, and without -b it blocks
# the client for as long as the popup is open. The pane to paste into is
# resolved by tmux-notepad itself (tmux display-message -p '#{pane_id}' inside
# a popup reports the pane the popup sits over), so no format expansion is
# needed here — which is just as well, since display-popup does not expand
# formats in the command it runs.
#
# Nothing in this file touches the network or the filesystem. tpm sources it
# synchronously while tmux is starting, so anything slow here would freeze the
# client; the key is bound to a wrapper that sorts out the binary on first use,
# inside the popup, where waiting is visible.

set -eu

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

option() {
  local value
  value="$(tmux show-option -gqv "$1")"
  [ -n "$value" ] || value="$2"
  printf '%s' "$value"
}

# display-popup hands the command to /bin/sh, so a plugin path with a space in
# it has to survive that round trip.
shell_quote() {
  case "$1" in
    *[!A-Za-z0-9_./-]*) printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")" ;;
    *) printf '%s' "$1" ;;
  esac
}

KEY="$(option '@notepad-key' 'n')"
WIDTH="$(option '@notepad-width' '80%')"
HEIGHT="$(option '@notepad-height' '80%')"
# Deliberately defaults to empty rather than to 'tmux-notepad': a user who names
# a binary here means that one, and must never get the self-installing wrapper.
BIN="$(option '@notepad-command' '')"
[ -n "$BIN" ] || BIN="$PLUGIN_DIR/scripts/notepad-popup.sh"

tmux bind-key "$KEY" display-popup -E -w "$WIDTH" -h "$HEIGHT" -T ' notepad ' "$(shell_quote "$BIN")"
